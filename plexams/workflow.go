package plexams

import (
	"context"
	"errors"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func initialWorkflow() []*model.Step {
	initialWorkflow := []*model.Step{
		{
			Name: "Prüfungen und Dozierende mit `plexams zpa` vom ZPA importieren.",
		},
		{
			Name: "Prüfungen auswählen, die geplant werden müssen.",
		},
		{
			Name: "Primuss-Daten mit Makefile importieren.",
		},
		{
			Name: "Zuordnung ZPA-Prüfungen zu Primuss-Anmeldungen fixieren.",
		},
		{
			Name: "Alle Nachteilsausgleiche eingegeben?",
		},
	}

	for i, s := range initialWorkflow {
		s.Number = i + 1
	}

	return initialWorkflow
}

func (p *Plexams) GetWorkflow(ctx context.Context) ([]*model.Step, error) {
	return p.dbClient.GetWorkflow(ctx)
}

func (p *Plexams) InitWorkflow(ctx context.Context) ([]*model.Step, error) {
	workflow, err := p.dbClient.GetWorkflow(ctx)

	if errors.Is(err, db.ErrNoWorkflowInitiated) {
		err = p.setWorkflow(ctx, initialWorkflow())
		if err != nil {
			return nil, err
		}
		return p.dbClient.GetWorkflow(ctx)
	}

	if err != nil {
		log.Error().Err(err).Msg("unexpected error while trying to get the workflow from the db")
		return nil, err
	}

	return workflow, err
}

func (p *Plexams) setWorkflow(ctx context.Context, workflow []*model.Step) error {
	err := p.dbClient.SetWorkflow(ctx, workflow)

	if err != nil {
		log.Error().Err(err).Msg("unexpected error while trying to set the workflow")
		return err
	}

	return nil
}
