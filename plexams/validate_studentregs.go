package plexams

import (
	"context"
	"fmt"
	"strings"
	"time"

	set "github.com/deckarep/golang-set/v2"
	"github.com/logrusorgru/aurora"
	"github.com/rs/zerolog/log"
	"github.com/theckman/yacspin"
)

// TODO: all planned_rooms okay? especially after moving an exam? check room -> slot -> ancode sameslot?
func (p *Plexams) ValidateStudentRegs() error {
	ctx := context.Background()
	cfg := yacspin.Config{
		Frequency:         100 * time.Millisecond,
		CharSet:           yacspin.CharSets[69],
		Suffix:            aurora.Sprintf(aurora.Cyan(" validating student regs")),
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

	studentRegs, err := p.dbClient.StudentRegsPerStudentPlanned(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get student regs")
	}

	spinner.Message(aurora.Sprintf(aurora.Yellow(" validating only regs from one program per student")))
	for _, studentReg := range studentRegs {
		programs := set.NewSet[string]()
		for _, reg := range studentReg.RegsWithProgram {
			programs.Add(reg.Program)
		}
		if programs.Cardinality() > 1 {
			var sb strings.Builder
			for _, reg := range studentReg.RegsWithProgram {
				zpaExam, err := p.dbClient.GetZpaExamByAncode(ctx, reg.Reg)
				if err != nil {
					log.Error().Err(err).Int("ancode", reg.Reg).
						Msg("cannot get zpa exam for student reg")
					continue
				}
				sb.WriteString(fmt.Sprintf("%s/%d: %s (%s)\n", reg.Program, zpaExam.AnCode, zpaExam.Module, zpaExam.MainExamer))
			}

			validationMessages = append(validationMessages, aurora.Sprintf(
				aurora.Red("regs from more than one program for student %s (%s/%s): %v\n%s"),
				aurora.Magenta(studentReg.Name),
				aurora.Cyan(studentReg.Program), aurora.Cyan(studentReg.Mtknr),
				aurora.Yellow(programs.ToSlice()),
				aurora.Yellow(sb.String()),
			))
		}
	}

	if len(validationMessages) > 0 {
		spinner.StopFailMessage(aurora.Sprintf(aurora.Red("%d problems"),
			len(validationMessages)))
		err = spinner.StopFail()
		if err != nil {
			log.Debug().Err(err).Msg("cannot stop spinner")
		}
		for _, msg := range validationMessages {
			fmt.Printf("%s\n", msg)
		}

	} else {
		spinner.StopMessage(aurora.Sprintf(aurora.Green("%d student registrations are okay"),
			len(studentRegs)))
		err = spinner.Stop()
		if err != nil {
			log.Debug().Err(err).Msg("cannot stop spinner")
		}
	}

	return nil
}
