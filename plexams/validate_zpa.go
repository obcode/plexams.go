package plexams

import (
	"context"

	"github.com/gookit/color"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) ValidateZPADateTimes() error {
	if err := p.SetZPA(); err != nil {
		return err
	}

	exams := p.zpa.client.GetExams()
	examsMap := make(map[int]*model.ZPAExam)

	for _, exam := range exams {
		examsMap[exam.AnCode] = exam
	}

	plannedExams, err := p.ExamsInPlan(context.Background())
	if err != nil {
		return err
	}

	problems := 0

	for _, plannedExam := range plannedExams {
		zpaExam := examsMap[plannedExam.Exam.Ancode]
		delete(examsMap, plannedExam.Exam.Ancode)

		plannedExamDate := plannedExam.Slot.Starttime.Local().Format("2006-01-02")
		plannedExamStarttime := plannedExam.Slot.Starttime.Local().Format("15:04:05")

		if zpaExam.Date != plannedExamDate ||
			zpaExam.Starttime != plannedExamStarttime {
			problems++
			color.Red.Printf("wrong date for %d. %s: %s\nwant: %s %s\ngot:  %s %s\n",
				plannedExam.Exam.Ancode, plannedExam.Exam.ZpaExam.MainExamer, plannedExam.Exam.ZpaExam.Module,
				plannedExamDate, plannedExamStarttime,
				zpaExam.Date, zpaExam.Starttime)
		}
	}

	if problems == 0 {
		color.Green.Printf("all %d planned exams in zpa with correct date/time\n", len(plannedExams))
	}

	problems = 0

	for _, zpaExam := range examsMap {
		if zpaExam.Date != "-" || zpaExam.Starttime != "-" {
			problems++
			color.Red.Printf("exam %d. %s: %s has date %s %s, but should not",
				zpaExam.AnCode, zpaExam.MainExamer, zpaExam.Module,
				zpaExam.Date, zpaExam.Starttime)
		}
	}

	if problems == 0 {
		color.Green.Printf("all %d not planned exams in zpa without date/time\n", len(examsMap))
	}

	return nil
}

func (p *Plexams) ValidateZPARooms() error {
	plannedExamsFromZPA, err := p.zpa.client.GetPlannedExams()
	if err != nil {
		return err
	}

	plannedExams, err := p.ExamsInPlan(context.Background())
	if err != nil {
		return err
	}

	problems := 0

	// check if plexams data is on zpa
	for _, plannedExam := range plannedExams {
		roomsForAncode, err := p.dbClient.RoomsForAncode(context.Background(), plannedExam.Exam.Ancode)
		if err != nil {
			log.Error().Err(err).Int("ancode", plannedExam.Exam.Ancode).Msg("cannot get planned rooms for ancode")
		}
		for _, room := range roomsForAncode {
			if room.RoomName == "No Room" {
				continue
			}
			found := false
			for _, zpaExam := range plannedExamsFromZPA {
				if room.Ancode == zpaExam.Ancode &&
					room.RoomName == zpaExam.RoomName &&
					room.Duration == zpaExam.Duration &&
					room.Handicap == zpaExam.IsHandicap &&
					room.Reserve == zpaExam.IsReserve &&
					room.SeatsPlanned == zpaExam.Number {
					found = true
					break
				}
			}
			if !found {
				problems++
				color.Red.Printf("room not found in ZPA\n   %+v\n", room)
			}
		}

	}

	if problems == 0 {
		color.Green.Println("all rooms planned found in zpa")
	}

	// TODO: check if zpa data is in plexams
	// for _, zpaExam := range plannedExamsFromZPA {

	// }

	return nil
}
