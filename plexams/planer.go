package plexams

import (
	"context"
	"fmt"
	"strings"

	"github.com/obcode/plexams.go/graph/model"
)

// GetPlaner returns the current planner: the stored overrides (name/email + optional
// testMail/cc/noreplyMail/noreplyName) together with the resolved effective values and the
// derived default mail, so the GUI can show both the override inputs and their placeholders.
func (p *Plexams) GetPlaner(ctx context.Context) (*model.Planer, error) {
	return p.planerModel(), nil
}

// planerModel builds the GraphQL Planer from the in-memory planner and the sender's resolved
// effective values.
func (p *Plexams) planerModel() *model.Planer {
	return &model.Planer{
		Name:                 p.planer.Name,
		Email:                p.planer.Email,
		TestMail:             nilIfEmpty(p.planer.TestMail),
		Cc:                   nilIfEmpty(p.planer.Cc),
		NoreplyMail:          nilIfEmpty(p.planer.NoreplyMail),
		NoreplyName:          nilIfEmpty(p.planer.NoreplyName),
		DefaultMail:          p.sender.DefaultMail(),
		EffectiveTestMail:    p.sender.EffectiveTestMail(),
		EffectiveCc:          p.sender.EffectiveCc(),
		EffectiveNoreplyMail: p.sender.EffectiveNoreplyMail(),
		EffectiveNoreplyName: p.sender.EffectiveNoreplyName(),
	}
}

// applyPlaner sets the running planner identity and overrides and keeps the mail sender in
// sync. The sender caches its own copy, so assigning p.planer alone would leave the From
// address and the override resolution stale.
func (p *Plexams) applyPlaner(planer *model.Planer) {
	p.planer = &Planer{
		Name:        planer.Name,
		Email:       planer.Email,
		TestMail:    deref(planer.TestMail),
		Cc:          deref(planer.Cc),
		NoreplyMail: deref(planer.NoreplyMail),
		NoreplyName: deref(planer.NoreplyName),
	}
	if p.sender != nil {
		p.sender.SetPlaner(p.planer.Name, p.planer.Email,
			p.planer.TestMail, p.planer.Cc, p.planer.NoreplyMail, p.planer.NoreplyName)
	}
}

// SetPlaner stores the planner (name/email + optional sender-identity overrides) in the
// global DB and applies it to the running instance. Blank overrides are stored as unset so
// the derived defaults apply.
func (p *Plexams) SetPlaner(ctx context.Context, name, email string, testMail, cc, noreplyMail, noreplyName *string) (*model.Planer, error) {
	name = strings.TrimSpace(name)
	email = strings.TrimSpace(email)
	if name == "" || email == "" {
		return nil, fmt.Errorf("name and email are required")
	}
	planer := &model.Planer{
		Name:        name,
		Email:       email,
		TestMail:    cleanPtr(testMail),
		Cc:          cleanPtr(cc),
		NoreplyMail: cleanPtr(noreplyMail),
		NoreplyName: cleanPtr(noreplyName),
	}
	if err := p.dbClient.SavePlaner(ctx, planer); err != nil {
		return nil, err
	}
	p.applyPlaner(planer)
	return p.planerModel(), nil
}

// DryRunTestMailStatus reports where dry-run mails currently go and whether a session
// override deviates from the configured default (so the GUI can show a warning banner).
func (p *Plexams) DryRunTestMailStatus() *model.DryRunTestMailStatus {
	override, has := p.sender.DryRunOverride()
	def := p.sender.EffectiveTestMail()
	status := &model.DryRunTestMailStatus{
		Current:    p.sender.DryRunRecipient(),
		Default:    def,
		Overridden: has && override != def,
	}
	if has {
		status.Override = &override
	}
	return status
}

// SetDryRunTestMail overrides the dry-run recipient for this session only (Probeläufe page).
// An empty/blank email resets to the configured default.
func (p *Plexams) SetDryRunTestMail(ctx context.Context, email string) (*model.DryRunTestMailStatus, error) {
	p.sender.SetDryRunOverride(email)
	return p.DryRunTestMailStatus(), nil
}

// ResetDryRunTestMail clears the session dry-run recipient override.
func (p *Plexams) ResetDryRunTestMail(ctx context.Context) (*model.DryRunTestMailStatus, error) {
	p.sender.ResetDryRunOverride()
	return p.DryRunTestMailStatus(), nil
}

// nilIfEmpty returns nil for a blank string, else a pointer to it — for optional GraphQL fields.
func nilIfEmpty(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}

// cleanPtr trims an optional string; a blank value becomes nil (unset override).
func cleanPtr(s *string) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil
	}
	return &t
}

// deref returns the pointed-to string, or "" when nil.
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
