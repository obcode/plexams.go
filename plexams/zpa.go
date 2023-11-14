package plexams

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
)

func (p *Plexams) GetZPAExam(ctx context.Context, ancode int) (*model.ZPAExam, error) {
	return p.dbClient.GetZpaExamByAncode(ctx, ancode)
}
