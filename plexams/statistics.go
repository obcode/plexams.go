package plexams

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"
)

func (p *Plexams) PrintStatistics() error {
	ctx := context.Background()

	// - Gesamtzahl der Anmeldungen aller Studierenden (auch die, die aus anderen Fakultäten kommen) auf alle von uns angebotenen Prüfungen.

	// - Gesamtzahl der Anmeldungen aller unserer Studierenden (nur FK07)

	// - Zahl aller von uns angebotenen Prüfungen im Sommersemester 2023
	zpaExamsAll, err := p.dbClient.GetZPAExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get ZPA exams")
	}
	zpaExamsTpPlan, err := p.dbClient.GetZPAExamsToPlan(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get ZPA exams")
	}

	fmt.Printf("- Zahl aller von uns angebotenen Prüfungen im Sommersemester 2023: %d, davon %d im Prüfungszeitraum\n",
		len(zpaExamsAll), len(zpaExamsTpPlan))

	return nil
}
