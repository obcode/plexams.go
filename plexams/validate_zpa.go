package plexams

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gookit/color"
	"github.com/logrusorgru/aurora"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"github.com/theckman/yacspin"
)

func (p *Plexams) ValidateZPADateTimes() error {
	cfg := yacspin.Config{
		Frequency:         100 * time.Millisecond,
		CharSet:           yacspin.CharSets[69],
		Suffix:            aurora.Sprintf(aurora.Cyan(" validating zpa dates and times")),
		SuffixAutoColon:   true,
		StopCharacter:     "✓",
		StopColors:        []string{"fgGreen"},
		StopFailMessage:   "error",
		StopFailCharacter: "✗",
		StopFailColors:    []string{"fgRed"},
	}

	spinner, err := yacspin.New(cfg)
	if err != nil {
		log.Debug().Err(err).Msg("cannot create spinner")
	}
	err = spinner.Start()
	if err != nil {
		log.Debug().Err(err).Msg("cannot start spinner")
	}

	validationMessages := make([]string, 0)

	spinner.Message(aurora.Sprintf(aurora.Yellow("fetching exams from ZPA")))
	if err := p.SetZPA(); err != nil {
		return err
	}

	exams := p.zpa.client.GetExams()
	examsMap := make(map[int]*model.ZPAExam)

	for _, exam := range exams {
		examsMap[exam.AnCode] = exam
	}

	spinner.Message(aurora.Sprintf(aurora.Yellow("fetching planned exams from db")))
	plannedExams, err := p.PlannedExams(context.Background())
	if err != nil {
		return err
	}

	notPlannedByMe := 0

	for _, plannedExam := range plannedExams {
		spinner.Message(aurora.Sprintf(aurora.Yellow("checking exam %d. %s (%s)"),
			plannedExam.Ancode, plannedExam.ZpaExam.Module, plannedExam.ZpaExam.MainExamer))
		zpaExam := examsMap[plannedExam.ZpaExam.AnCode]
		delete(examsMap, plannedExam.ZpaExam.AnCode)

		shouldHaveNoTimeAndDate := false
		if plannedExam.Constraints != nil && plannedExam.Constraints.NotPlannedByMe {
			shouldHaveNoTimeAndDate = true
			notPlannedByMe++
		}

		if zpaExam == nil {
			log.Error().Int("ancode", plannedExam.ZpaExam.AnCode).Str("examer", plannedExam.ZpaExam.MainExamer).
				Str("module", plannedExam.ZpaExam.Module).Msg("zpa exam not found")
			continue
		}

		plannedExamDate := "-"
		plannedExamStarttime := "-"
		if !shouldHaveNoTimeAndDate && plannedExam.PlanEntry != nil {
			plannedExamDate = plannedExam.PlanEntry.Starttime.Local().Format("2006-01-02")
			plannedExamStarttime = plannedExam.PlanEntry.Starttime.Local().Format("15:04:05")
		}

		if zpaExam.Date != plannedExamDate ||
			zpaExam.Starttime != plannedExamStarttime {
			validationMessages = append(validationMessages, aurora.Sprintf(
				aurora.Red("wrong date for %d. %s (%s), want: %s %s, got: %s %s"),
				aurora.Cyan(zpaExam.AnCode), aurora.Cyan(zpaExam.Module), aurora.Cyan(zpaExam.MainExamer),
				aurora.Green(plannedExamDate), aurora.Green(plannedExamStarttime),
				aurora.Magenta(zpaExam.Date), aurora.Magenta(zpaExam.Starttime),
			))
		}
	}

	for _, zpaExam := range examsMap {
		if zpaExam.Date != "-" || zpaExam.Starttime != "-" {
			validationMessages = append(validationMessages, aurora.Sprintf(
				aurora.Red("exam %d. %s (%s) has date %s %s, but should not be planned"),
				aurora.Cyan(zpaExam.AnCode), aurora.Cyan(zpaExam.Module), aurora.Cyan(zpaExam.MainExamer),
				aurora.Magenta(zpaExam.Date), aurora.Magenta(zpaExam.Starttime)))
		}
	}

	if len(validationMessages) > 0 {
		spinner.StopFailMessage(aurora.Sprintf(aurora.Red("%d problems found"), len(validationMessages)))
		err = spinner.StopFail()
		if err != nil {
			log.Debug().Err(err).Msg("cannot stop spinner")
		}
		for _, msg := range validationMessages {
			fmt.Printf("    ↪ %s\n", msg)
		}

	} else {
		spinner.StopMessage(aurora.Sprintf(
			aurora.Green("%d planned exams (%d not planned by me) & %d not planned are correct"),
			len(plannedExams), notPlannedByMe, len(examsMap)))
		err = spinner.Stop()
		if err != nil {
			log.Debug().Err(err).Msg("cannot stop spinner")
		}
	}

	return nil
}

func (p *Plexams) ValidateZPARooms() error {
	cfg := yacspin.Config{
		Frequency:         100 * time.Millisecond,
		CharSet:           yacspin.CharSets[69],
		Suffix:            aurora.Sprintf(aurora.Cyan(" validating zpa rooms")),
		SuffixAutoColon:   true,
		StopCharacter:     "✓",
		StopColors:        []string{"fgGreen"},
		StopFailMessage:   "error",
		StopFailCharacter: "✗",
		StopFailColors:    []string{"fgRed"},
	}

	spinner, err := yacspin.New(cfg)
	if err != nil {
		log.Debug().Err(err).Msg("cannot create spinner")
	}
	err = spinner.Start()
	if err != nil {
		log.Debug().Err(err).Msg("cannot start spinner")
	}

	validationMessages := make([]string, 0)

	spinner.Message(aurora.Sprintf(aurora.Yellow("fetching exams from ZPA")))
	if err := p.SetZPA(); err != nil {
		return err
	}

	plannedExamsFromZPA, err := p.zpa.client.GetPlannedExams()
	if err != nil {
		return err
	}

	spinner.Message(aurora.Sprintf(aurora.Yellow("fetching planned exams from db")))
	plannedExams, err := p.PlannedExams(context.Background())
	if err != nil {
		return err
	}

	notPlannedByMe := 0
	roomsChecked := 0

	// check if plexams data is on zpa
	for _, plannedExam := range plannedExams {
		if plannedExam.Constraints != nil && plannedExam.Constraints.NotPlannedByMe {
			notPlannedByMe++
			continue
		}

		roomsForAncode, err := p.dbClient.PlannedRoomsForAncode(context.Background(), plannedExam.Ancode)
		if err != nil {
			log.Error().Err(err).Int("ancode", plannedExam.Ancode).Msg("cannot get planned rooms for ancode")
		}
		for _, room := range roomsForAncode {
			if room.RoomName == "No Room" {
				continue
			}
			roomsChecked++
			found := false
			for _, zpaExam := range plannedExamsFromZPA {
				if room.Ancode == zpaExam.Ancode &&
					roomNameOK(room.RoomName, zpaExam.RoomName) &&
					room.Duration == zpaExam.Duration &&
					room.Handicap == zpaExam.IsHandicap &&
					room.Reserve == zpaExam.IsReserve &&
					(len(room.StudentsInRoom) <= zpaExam.Number || // if more than one NTA in the room
						zpaExam.RoomName == "ONLINE") {
					found = true
					break
				}
			}
			if !found {
				validationMessages = append(validationMessages, aurora.Sprintf(
					aurora.Red("room %s for exam %d. %s (%s) not found in ZPA"),
					aurora.Magenta(room.RoomName),
					aurora.Cyan(plannedExam.Ancode), aurora.Cyan(plannedExam.ZpaExam.Module), aurora.Cyan(plannedExam.ZpaExam.MainExamer)))
			}
		}

	}

	if len(validationMessages) > 0 {
		spinner.StopFailMessage(aurora.Sprintf(aurora.Red("%d problems found"), len(validationMessages)))
		err = spinner.StopFail()
		if err != nil {
			log.Debug().Err(err).Msg("cannot stop spinner")
		}
		for _, msg := range validationMessages {
			fmt.Printf("    ↪ %s\n", msg)
		}

	} else {
		spinner.StopMessage(aurora.Sprintf(
			aurora.Green("%d planned exams (%d not planned by me) with %d room entries are correct"),
			len(plannedExams), notPlannedByMe, roomsChecked))
		err = spinner.Stop()
		if err != nil {
			log.Debug().Err(err).Msg("cannot stop spinner")
		}
	}

	// TODO: check if zpa data is in plexams
	// for _, zpaExam := range plannedExamsFromZPA {

	// }

	return nil
}

func (p *Plexams) ValidateZPAInvigilators() error {
	ctx := context.Background()
	plannedExamsFromZPA, err := p.zpa.client.GetPlannedExams()
	if err != nil {
		return err
	}

	plannedExams, err := p.ExamsInPlan(ctx)
	if err != nil {
		return err
	}

	problems := 0

	// check if plexams data is on zpa
	for _, plannedExam := range plannedExams {
		roomsForAncode, err := p.dbClient.RoomsForAncode(ctx, plannedExam.Exam.Ancode)
		if err != nil {
			log.Error().Err(err).Int("ancode", plannedExam.Exam.Ancode).Msg("cannot get planned rooms for ancode")
			problems++
		}
		reserveInvigilator, err := p.GetInvigilatorInSlot(ctx, "reserve", plannedExam.Slot.DayNumber, plannedExam.Slot.SlotNumber)
		if err != nil {
			log.Error().Err(err).Int("day", plannedExam.Slot.DayNumber).Int("slot", plannedExam.Slot.SlotNumber).
				Msg("cannot get reserve invigilator for slot")
			problems++
		}
		for _, room := range roomsForAncode {
			if room.RoomName == "No Room" {
				continue
			}
			invigilator, err := p.GetInvigilatorInSlot(ctx, room.RoomName, plannedExam.Slot.DayNumber, plannedExam.Slot.SlotNumber)
			if err != nil {
				log.Error().Err(err).Int("day", plannedExam.Slot.DayNumber).Int("slot", plannedExam.Slot.SlotNumber).
					Msg("cannot get reserve invigilator for slot")
				problems++
			}
			found := false
			for _, zpaExam := range plannedExamsFromZPA {
				if room.Ancode == zpaExam.Ancode &&
					roomNameOK(room.RoomName, zpaExam.RoomName) &&
					zpaExam.ReserveSupervisor == reserveInvigilator.Shortname &&
					zpaExam.Supervisor == invigilator.Shortname {
					found = true
					break
				}
			}
			if !found {
				problems++
				color.Red.Printf("supervisor or reserve supervisor not found in ZPA\n   %+v\n", room)
			}
		}

	}

	if problems == 0 {
		color.Green.Println("all invigilators planned found in zpa")
	}

	// TODO: check if zpa data is in plexams
	// for _, zpaExam := range plannedExamsFromZPA {

	// }

	return nil
}

func roomNameOK(roomPlexams, roomZPA string) bool {
	return roomPlexams == roomZPA ||
		(strings.HasPrefix(roomPlexams, "ONLINE") && roomZPA == "ONLINE")
}
