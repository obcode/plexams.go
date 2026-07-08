package plexams

import (
	"context"
	"fmt"
	"strings"

	"github.com/obcode/plexams.go/graph/model"
)

// GetPlaner returns the current planner (name + email).
func (p *Plexams) GetPlaner(ctx context.Context) (*model.Planer, error) {
	return &model.Planer{Name: p.planer.Name, Email: p.planer.Email}, nil
}

// applyPlaner sets the running planner identity and keeps the mail sender's From
// address in sync. The sender caches its own copy of the planner, so assigning
// p.planer alone would leave the From address stale (or empty, which fails as
// `"" <>` when a mail is built).
func (p *Plexams) applyPlaner(name, email string) {
	p.planer = &Planer{Name: name, Email: email}
	if p.sender != nil {
		p.sender.SetPlaner(name, email)
	}
}

// SetPlaner stores the planner in the global DB and applies it to the running
// instance.
func (p *Plexams) SetPlaner(ctx context.Context, name, email string) (*model.Planer, error) {
	name = strings.TrimSpace(name)
	email = strings.TrimSpace(email)
	if name == "" || email == "" {
		return nil, fmt.Errorf("name and email are required")
	}
	planer := &model.Planer{Name: name, Email: email}
	if err := p.dbClient.SavePlaner(ctx, planer); err != nil {
		return nil, err
	}
	p.applyPlaner(name, email)
	return planer, nil
}
