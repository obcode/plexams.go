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

func (p *Plexams) NtasWithRegs(ctx context.Context) ([]*model.Student, error) {
	return p.dbClient.NtasWithRegs(ctx)
}

// Deprecated: use StudentByMtknr
func (p *Plexams) Nta(ctx context.Context, mtknr string) (*model.NTAWithRegs, error) {
	return p.dbClient.NtaWithRegs(ctx, mtknr)
}

func (p *Plexams) NtaByMtknr(ctx context.Context, mtknr string) (*model.NTA, error) {
	return p.dbClient.Nta(ctx, mtknr)
}
