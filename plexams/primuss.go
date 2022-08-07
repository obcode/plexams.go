package plexams

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
)

func (p *Plexams) PrimussExams(ctx context.Context) ([]*model.PrimussExamByGroup, error) {
	return p.dbClient.GetPrimussExams(ctx)
}
