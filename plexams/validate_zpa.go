package plexams

import (
	"context"
	"fmt"

	"github.com/gookit/color"
	"github.com/obcode/plexams.go/graph/model"
)

func (p *Plexams) ValidateZPA(withRooms, withInvigilators bool) error {
	if err := p.SetZPA(); err != nil {
		return err
	}

	if withInvigilators {
		withRooms = true
	}

	// check only times
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

	if withRooms {
		plannedExamsFromZPA, err := p.zpa.client.GetPlannedExams()
		if err != nil {
			return err
		}

		fmt.Printf("%+v\n", plannedExamsFromZPA)
	}

	return nil
}
