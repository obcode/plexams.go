package plexams

import (
	"context"
	"fmt"
	"strings"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/principal"
)

// MyAccount returns the caller's own account: identity (from the DB user, or the ctx
// principal for the local dev user when auth is disabled), the effective and ZPA
// Kürzel, and the Jira-token status.
func (p *Plexams) MyAccount(ctx context.Context) (*model.MyAccount, error) {
	email, err := p.principalEmail(ctx)
	if err != nil {
		return nil, err
	}

	name, role, shortOverride := "", model.RoleViewer, ""
	user, err := p.dbClient.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if user != nil {
		name, role, shortOverride = user.Name, user.Role, user.Shortname
	} else if u := principal.UserFromContext(ctx); u != nil {
		// local dev user (auth disabled) is not in the allow-list
		name, role = u.Name, u.Role
	}

	zpaShort := p.ShortnameFromZPA(ctx, email)
	effective := shortOverride
	if effective == "" {
		effective = zpaShort
	}

	set, updatedAt, err := p.myJiraStatus(ctx, email)
	if err != nil {
		return nil, err
	}

	return &model.MyAccount{
		Email:              email,
		Name:               name,
		Role:               role,
		Shortname:          effective,
		ShortnameFromZpa:   zpaShort,
		JiraTokenSet:       set,
		JiraTokenUpdatedAt: updatedAt,
	}, nil
}

// SetMyShortname sets the caller's Kürzel override (empty resets to the ZPA default).
// Load-modify-save so the admin-managed fields (name/role) are preserved.
func (p *Plexams) SetMyShortname(ctx context.Context, shortname string) error {
	email, err := p.principalEmail(ctx)
	if err != nil {
		return err
	}
	shortname = strings.TrimSpace(shortname)

	user, err := p.dbClient.GetUserByEmail(ctx, email)
	if err != nil {
		return err
	}
	if user == nil {
		// local dev user (auth disabled) not in the allow-list — synthesize from ctx
		u := principal.UserFromContext(ctx)
		if u == nil {
			return fmt.Errorf("no authenticated user in context")
		}
		user = &model.User{Email: email, Name: u.Name, Role: u.Role}
	}
	user.Shortname = shortname
	return p.dbClient.SaveUser(ctx, user)
}

// ShortnameFromZPA returns the Kürzel of the ZPA teacher matching the email, or ""
// when there is no match (or no active-semester teachers).
func (p *Plexams) ShortnameFromZPA(ctx context.Context, email string) string {
	if p.dbClient == nil || strings.TrimSpace(email) == "" {
		return ""
	}
	teacher, err := p.dbClient.GetTeacherByEmail(ctx, email)
	if err != nil || teacher == nil {
		return ""
	}
	return teacher.Shortname
}
