package plexams

import (
	"context"
	"fmt"

	set "github.com/deckarep/golang-set/v2"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) PrintStudentInfo(name string, long, zpa bool) error {
	ctx := context.Background()
	students, err := p.StudentsByName(context.TODO(), name) // nolint
	if err != nil {
		return err
	}
	for _, student := range students {
		if !long {
			fmt.Printf("%s (%s, %s%s): regs %v", student.Name, student.Mtknr, student.Program, student.Group, student.Regs)
			if student.Nta != nil {
				fmt.Printf(", NTA: %s\n", student.Nta.Compensation)
			} else {
				fmt.Println()
			}
		} else {
			fmt.Printf("%s (%s, %s%s)", student.Name, student.Mtknr, student.Program, student.Group)
			if student.Nta != nil {
				fmt.Printf(", NTA: %s\n", student.Nta.Compensation)
			} else {
				fmt.Println()
			}
			for _, ancode := range student.Regs {
				examsToPlan, err := p.GetZpaExamsToPlan(ctx)
				if err != nil {
					log.Error().Err(err).Msg("cannot get exams to plan")
				}
				examsToPlanSet := set.NewSet[int]()
				for _, exam := range examsToPlan {
					examsToPlanSet.Add(exam.AnCode)
				}
				if examsToPlanSet.Contains(ancode) {
					exam, err := p.GeneratedExam(ctx, ancode)
					if err != nil {
						log.Debug().Err(err).Int("ancode", ancode).Msg("cannot get exam")
					}
					if exam != nil {
						fmt.Printf("  - %d. %s: %s (%s)\n", ancode, exam.ZpaExam.MainExamer, exam.ZpaExam.Module, exam.ZpaExam.ExamTypeFull)
					} else {
						fmt.Printf("  - %d: not found\n", ancode)
					}
				} else {
					exam, err := p.GetZPAExam(ctx, ancode)
					if err != nil {
						log.Debug().Err(err).Int("ancode", ancode).Msg("exam not found")
					}
					fmt.Printf("  - %d. %s: %s not planned\n", ancode, exam.MainExamer, exam.Module)
				}

			}

		}
		if zpa {
			zpaStudents, err := p.GetStudents(context.TODO(), student.Mtknr)
			if err != nil {
				fmt.Printf("  -> Studierenden nicht im ZPA gefunden: %v\n", err)
			} else {
				switch len(zpaStudents) {
				case 0:
					fmt.Println("  -> Studierenden nicht im ZPA gefunden")
				case 1:
					fmt.Printf("  -> %+v\n", zpaStudents[0])
				case 2:
					fmt.Println("  -> mehrere Studierende mit selber MtkNr gefunden")
					for _, stud := range zpaStudents {
						fmt.Printf("      -> %+v\n", stud)
					}
				}
			}
		}
	}
	return nil
}
