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
	studentRegs, err := p.dbClient.StudentRegsPerStudentPlanned(ctx)
	if err != nil {
		return err
	}

	regs := 0
	for _, studentReg := range studentRegs {
		regs += len(studentReg.Regs)
	}

	fmt.Printf("- %s mit insgesamt %s auf unsere Prüfungen\n",
		pluralN(len(studentRegs), "Studierende:r", "Studierende"), pluralN(regs, "Anmeldung", "Anmeldungen"))

	studentRegsFK07 := 0
	regsFK07 := 0
STUDENTREG:
	for _, studentReg := range studentRegs {
		for _, program := range p.zpa.fk07programs {
			if program == studentReg.Program {
				studentRegsFK07++
				regsFK07 += len(studentReg.Regs)
				continue STUDENTREG
			}
		}
	}

	// - Gesamtzahl der Anmeldungen aller unserer Studierenden (nur FK07)
	fmt.Printf("- %s der FK07 mit insgesamt %s auf unsere Prüfungen\n",
		pluralN(studentRegsFK07, "Studierende:r", "Studierende"), pluralN(regsFK07, "Anmeldung", "Anmeldungen"))

	// - Zahl aller von uns angebotenen Prüfungen im Sommersemester 2023
	zpaExamsAll, err := p.dbClient.GetZPAExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get ZPA exams")
	}
	zpaExamsToPlan, err := p.dbClient.GetZPAExamsToPlan(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get ZPA exams")
	}

	fmt.Printf("- %s, davon %d im Prüfungszeitraum\n",
		pluralN(len(zpaExamsAll), "von uns angebotene Prüfung", "von uns angebotene Prüfungen"), len(zpaExamsToPlan))

	return nil
}
