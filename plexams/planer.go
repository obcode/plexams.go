package plexams

import (
	"context"
	"fmt"
	"strings"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// GetPlaner returns the planner in force for the current semester: the resolved name/email,
// the semester's own sender overrides, and the effective values those override resolve to,
// so the GUI can show both the override inputs and their placeholders.
func (p *Plexams) GetPlaner(ctx context.Context) (*model.Planer, error) {
	return p.planerModel(), nil
}

// planerModel builds the GraphQL Planer from the resolved planner and the sender's effective
// values.
func (p *Plexams) planerModel() *model.Planer {
	return &model.Planer{
		Name:                 p.planer.Name,
		Email:                p.planer.Email,
		Inherited:            p.planer.Inherited,
		DefaultName:          p.defaultPlaner.Name,
		DefaultEmail:         p.defaultPlaner.Email,
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

// resolvePlaner recomputes the running planner from the config default and the current
// semester's override, and keeps the mail sender in sync. It must be called whenever either
// side can have changed: at start, after a semester switch, and after the override is
// edited. The sender caches its own copy, so assigning p.planer alone would leave the From
// address and the override resolution stale.
//
// A DB error is logged and leaves the config default in force rather than failing the boot:
// an unreachable database is already reported everywhere else, and a server that refuses to
// start over the sender's display name helps nobody.
func (p *Plexams) resolvePlaner(ctx context.Context) {
	var override *db.SemesterPlaner
	if p.dbClient != nil {
		var err error
		if override, err = p.dbClient.GetSemesterPlaner(ctx); err != nil {
			log.Error().Err(err).Msg("cannot read the semester planer, falling back to the configured default")
			override = nil
		}
	}

	resolved := mergePlaner(*p.defaultPlaner, override)
	p.planer = &resolved
	if p.sender != nil {
		p.sender.SetPlaner(p.planer.Name, p.planer.Email,
			p.planer.TestMail, p.planer.Cc, p.planer.NoreplyMail, p.planer.NoreplyName)
	}
}

// mergePlaner lays a semester's override over the configured default. A nil override
// (semester has none) yields the default unchanged.
func mergePlaner(def Planer, override *db.SemesterPlaner) Planer {
	resolved := def
	resolved.Inherited = true
	if override == nil {
		return resolved
	}

	// Name and email are one identity -- either the semester has its own or it inherits
	// both. The four sender overrides are independent settings and do merge field by
	// field.
	if override.Name != nil && override.Email != nil {
		resolved.Name = *override.Name
		resolved.Email = *override.Email
		resolved.Inherited = false
	}
	// The config has no counterpart for these four -- their middle tier is smtp.*,
	// resolved inside the Sender -- so an unset one stays empty and the Sender falls
	// through to smtp.* and then to its derived default.
	resolved.TestMail = deref(override.TestMail)
	resolved.Cc = deref(override.Cc)
	resolved.NoreplyMail = deref(override.NoreplyMail)
	resolved.NoreplyName = deref(override.NoreplyName)

	return resolved
}

// SetSemesterPlaner stores this semester's planner override and applies it to the running
// instance.
//
// name and email are one identity: pass both to give the semester its own planner, or
// neither to inherit the configured default. Half of one would send as "Oliver Braun
// <someone.else@hm.edu>", so it is refused here as well as by the table constraint. The four
// sender overrides are independent; blank means unset, i.e. fall back to smtp.* and then to
// the derived default.
func (p *Plexams) SetSemesterPlaner(ctx context.Context, name, email, testMail, cc, noreplyMail, noreplyName *string) (*model.Planer, error) {
	cleanName, cleanEmail := cleanPtr(name), cleanPtr(email)
	if (cleanName == nil) != (cleanEmail == nil) {
		return nil, fmt.Errorf("name and email belong together: set both to give this semester its own planner, or neither to use the configured default")
	}

	if err := p.dbClient.SaveSemesterPlaner(ctx, &db.SemesterPlaner{
		Name:        cleanName,
		Email:       cleanEmail,
		TestMail:    cleanPtr(testMail),
		Cc:          cleanPtr(cc),
		NoreplyMail: cleanPtr(noreplyMail),
		NoreplyName: cleanPtr(noreplyName),
	}); err != nil {
		return nil, err
	}

	p.resolvePlaner(ctx)
	return p.planerModel(), nil
}

// ResetSemesterPlaner drops this semester's override, so the configured default applies
// again.
func (p *Plexams) ResetSemesterPlaner(ctx context.Context) (*model.Planer, error) {
	if err := p.dbClient.DeleteSemesterPlaner(ctx); err != nil {
		return nil, err
	}
	p.resolvePlaner(ctx)
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
