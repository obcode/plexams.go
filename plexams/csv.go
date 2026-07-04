package plexams

import (
	"context"
	"os"

	set "github.com/deckarep/golang-set/v2"
	"github.com/jszwec/csvutil"
	"github.com/obcode/plexams.go/plexams/csvgen"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) CsvForProgram(program, filename string) error {
	ctx := context.Background()
	exams, err := p.PlannedExamsForProgram(ctx, program, true)
	if err != nil {
		log.Error().Err(err).Str("program", program).Msg("cannot get planned exams for program")
		return err
	}

	b, err := csvutil.Marshal(csvgen.ProgramRows(exams, program, p.getSlotTime))
	if err != nil {
		log.Error().Err(err).Msg("error when marshaling to csv")
	}

	return os.WriteFile(filename, b, 0644)
}

func (p *Plexams) CsvForEXaHM(filename string) error {
	ctx := context.Background()
	exams, err := p.PlannedExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get planned exams")
		return err
	}

	b, err := csvutil.Marshal(csvgen.ExahmRows(exams, p.getSlotTime))
	if err != nil {
		log.Error().Err(err).Msg("error when marshaling to csv")
	}

	return os.WriteFile(filename, b, 0644)
}

type CsvLBARepeater struct {
	Ancode             int    `csv:"Ancode"`
	Module             string `csv:"Modul"`
	MainExamer         string `csv:"Erstprüfender"`
	EmailMainExamer    string `csv:"E-Mail Erstprüfender"`
	ExamDate           string `csv:"Termin"`
	Invigilators       string `csv:"Aufsichten"`
	EmailsInvigilators string `csv:"E-Mails Aufsichten"`
}

func (p *Plexams) CsvForLBARepeater(filename string) error {
	ctx := context.Background()
	plannedExams, err := p.PlannedExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get planned exams")
		return err
	}

	var csvEntries []CsvLBARepeater
	for _, exam := range plannedExams {
		if !exam.ZpaExam.IsRepeaterExam {
			continue
		}

		if exam.Constraints != nil && exam.Constraints.NotPlannedByMe {
			continue
		}

		mainExamer, err := p.GetTeacher(ctx, exam.ZpaExam.MainExamerID)
		if err != nil {
			log.Error().Err(err).Msg("cannot get main examiner")
			return err
		}

		if !mainExamer.IsLBA {
			continue
		}

		examDate := "fehlt"
		if exam.PlanEntry != nil {
			starttime := p.getSlotTime(exam.PlanEntry.DayNumber, exam.PlanEntry.SlotNumber)
			examDate = starttime.Format("02.01.06, 15:04 Uhr")
		}

		invigilators, invigilatorEmails := "", ""

		invigs := set.NewSet[int]()
		for _, room := range exam.PlannedRooms {
			invigilator, err := p.GetInvigilatorForRoom(ctx, room.RoomName, exam.PlanEntry.DayNumber, exam.PlanEntry.SlotNumber)
			if err != nil {
				log.Error().Err(err).Msg("cannot get invigilator")
				return err
			}
			if invigs.Contains(invigilator.ID) {
				continue
			}
			invigilators += invigilator.Shortname + ", "
			invigilatorEmails += invigilator.Email + ", "
			invigs.Add(invigilator.ID)
		}

		csvEntries = append(csvEntries, CsvLBARepeater{
			Ancode:             exam.Ancode,
			Module:             exam.ZpaExam.Module,
			MainExamer:         exam.ZpaExam.MainExamer,
			EmailMainExamer:    mainExamer.Email,
			ExamDate:           examDate,
			Invigilators:       invigilators,
			EmailsInvigilators: invigilatorEmails,
		})
	}

	b, err := csvutil.Marshal(csvEntries)
	if err != nil {
		log.Error().Err(err).Msg("error when marshaling to csv")
	}

	return os.WriteFile(filename, b, 0644)
}
