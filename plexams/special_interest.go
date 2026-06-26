package plexams

import (
	"context"
	"fmt"
	"strings"

	"github.com/obcode/plexams.go/graph/model"
)

// SpecialInterests returns all special-interest groups.
func (p *Plexams) SpecialInterests(ctx context.Context) ([]*model.SpecialInterest, error) {
	return p.dbClient.SpecialInterests(ctx)
}

// UpsertSpecialInterest creates or updates one special-interest group (key: name).
func (p *Plexams) UpsertSpecialInterest(ctx context.Context, si *model.SpecialInterest) (*model.SpecialInterest, error) {
	si.Name = strings.TrimSpace(si.Name)
	if si.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if err := p.dbClient.UpsertSpecialInterest(ctx, si); err != nil {
		return nil, err
	}
	return si, nil
}

// DeleteSpecialInterest removes one special-interest group by name.
func (p *Plexams) DeleteSpecialInterest(ctx context.Context, name string) (bool, error) {
	return p.dbClient.DeleteSpecialInterest(ctx, name)
}
