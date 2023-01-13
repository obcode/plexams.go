package plexams

import (
	"context"

	set "github.com/deckarep/golang-set/v2"
	"github.com/gookit/color"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) ValidateInvigilatorRequirements() error {
	ctx := context.Background()
	invigilationTodos, err := p.InvigilationTodos(ctx)
	if err != nil {
		return err
	}

	for _, invigilator := range invigilationTodos.Invigilators {
		log.Debug().Str("name", invigilator.Teacher.Shortname).Msg("checking constraints")

		// days ok
		for _, invigilationDay := range invigilator.Todos.InvigilationDays {
			for _, excludedDay := range invigilator.Requirements.ExcludedDays {
				if invigilationDay == excludedDay {
					color.Red.Printf("%s has invigilation on excluded day %d\n", invigilator.Teacher.Shortname, invigilationDay)
				}
			}
		}

		// wenn gleichzeitig Prüfung, dann nur self-invigilation
		exams, err := p.dbClient.PlannedExamsByMainExamer(ctx, invigilator.Teacher.ID)
		if err != nil {
			log.Error().Err(err).Str("name", invigilator.Teacher.Shortname).Msg("cannot get exams")
		}

		for _, exam := range exams {
			for _, invigilation := range invigilator.Todos.Invigilations {
				if exam.Slot.DayNumber == invigilation.Slot.DayNumber &&
					exam.Slot.SlotNumber == invigilation.Slot.SlotNumber {
					if invigilation.IsReserve {
						color.Red.Printf("%s has reserve invigilation during own exam %d. %s in slot (%d,%d)\n", invigilator.Teacher.Shortname,
							exam.Constraints.Ancode, exam.Exam.ZpaExam.Module, exam.Slot.DayNumber, exam.Slot.SlotNumber)
					}

					roomsForExam, err := p.dbClient.RoomsForAncode(ctx, exam.Exam.Ancode)
					rooms := set.NewSet[string]()
					for _, room := range roomsForExam {
						rooms.Add(room.RoomName)
					}

					if err != nil {
						log.Error().Err(err).Int("ancode", exam.Exam.Ancode).Msg("cannot get rooms for exam")
					} else {
						if rooms.Cardinality() != 1 {
							color.Red.Printf("%s has invigilation during own exam with more than one room: %d. %s in slot (%d,%d)\n", invigilator.Teacher.Shortname,
								exam.Constraints.Ancode, exam.Exam.ZpaExam.Module, exam.Slot.DayNumber, exam.Slot.SlotNumber)
						}
					}

				}
			}
		}

	}

	return nil
}
