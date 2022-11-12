package plexams

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
)

func (p *Plexams) GetStudentRegsPerAncodePlanned(ctx context.Context) ([]*model.StudentRegsPerAncode, error) {
	return p.dbClient.GetStudentRegsPerAncodePlanned(ctx)
}
