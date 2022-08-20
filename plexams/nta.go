package plexams

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
)

func (p *Plexams) AddNta(ctx context.Context, input model.NTAInput) (*model.NTA, error) {
	return p.dbClient.AddNta(ctx, model.NtaInputToNta(input))
}

func (p *Plexams) Ntas(ctx context.Context) ([]*model.NTA, error) {
	return p.dbClient.Ntas(ctx)
}
