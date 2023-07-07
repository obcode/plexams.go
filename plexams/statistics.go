package plexams

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"
)

func (p *Plexams) PrintStatistics() error {
	ctx := context.Background()

	// - Gesamtzahl der Anmeldungen aller Studierenden (auch die, die aus anderen Fakultäten kommen) auf alle von uns angebotenen Prüfungen.
	// Zähle
	studentRegs, err := p.dbClient.StudentRegsPerStudentAll(ctx)
	if err != nil {
		return err
	}

	regs := 0
	for _, studentReg := range studentRegs {
		regs += len(studentReg.Ancodes)
	}

	fmt.Printf("- %d Studierende mit insgesamt %d Anmeldungen auf unsere Prüfungen\n", len(studentRegs), regs)

	studentRegsFK07 := 0
	regsFK07 := 0
STUDENTREG:
	for _, studentReg := range studentRegs {
		for _, program := range p.zpa.fk07programs {
			if program == studentReg.Student.Program {
				studentRegsFK07++
				regsFK07 += len(studentReg.Ancodes)
				continue STUDENTREG
			}
		}
	}

	// - Gesamtzahl der Anmeldungen aller unserer Studierenden (nur FK07)
	fmt.Printf("- %d Studierende der FK07 mit insgesamt %d Anmeldungen auf unsere Prüfungen\n",
		studentRegsFK07, regsFK07)

	// - Zahl aller von uns angebotenen Prüfungen im Sommersemester 2023
	zpaExamsAll, err := p.dbClient.GetZPAExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get ZPA exams")
	}
	zpaExamsToPlan, err := p.dbClient.GetZPAExamsToPlan(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get ZPA exams")
	}

	fmt.Printf("- %d von uns angebotenen Prüfungen, davon %d im Prüfungszeitraum\n",
		len(zpaExamsAll), len(zpaExamsToPlan))

	return nil
}
