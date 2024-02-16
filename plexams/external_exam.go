package plexams

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
)

func (p *Plexams) AddExternalExam(ctx context.Context, primussExam *model.PrimussExam, duration int) error {
	return p.dbClient.AddExternalExam(ctx, &model.ExternalExam{
		Ancode:     primussExam.AnCode,
		Program:    primussExam.Program,
		Module:     primussExam.Module,
		MainExamer: primussExam.MainExamer,
		Duration:   duration,
	})
}

func (p *Plexams) ExternalExams(ctx context.Context) ([]*model.ExternalExam, error) {
	return p.dbClient.ExternalExams(ctx)
}
