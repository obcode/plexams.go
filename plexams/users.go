package plexams

import (
	"context"
	"fmt"
	"strings"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// Users are the login identities/roles for the server deployment (matched to the
// email the auth proxy supplies). They live in the global plexams DB and are the
// authorization allow-list — kept strictly separate from the planer (the shared
// email sender identity). See db/users.go and graph/auth.go.

// GetUsers returns the whole allow-list.
func (p *Plexams) GetUsers(ctx context.Context) ([]*model.User, error) {
	if p.dbClient == nil {
		return []*model.User{}, nil
	}
	return p.dbClient.GetUsers(ctx)
}

// GetUserByEmail looks a user up by email (nil when unknown).
func (p *Plexams) GetUserByEmail(ctx context.Context, email string) (*model.User, error) {
	if p.dbClient == nil {
		return nil, nil
	}
	return p.dbClient.GetUserByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
}

// SetUser upserts a user (email keyed, lower-cased). Admin surface for opening
// access to a wider circle with restricted rights.
func (p *Plexams) SetUser(ctx context.Context, email, name string, role model.Role) (*model.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	name = strings.TrimSpace(name)
	if email == "" || name == "" {
		return nil, fmt.Errorf("email and name are required")
	}
	if !role.IsValid() {
		return nil, fmt.Errorf("invalid role %q", role)
	}
	user := &model.User{Email: email, Name: name, Role: role}
	if err := p.dbClient.SaveUser(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

// RemoveUser deletes a user from the allow-list.
func (p *Plexams) RemoveUser(ctx context.Context, email string) (bool, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return false, fmt.Errorf("email is required")
	}
	if err := p.dbClient.DeleteUser(ctx, email); err != nil {
		return false, err
	}
	return true, nil
}

// SeedUsers seeds the allow-list from config (auth.seedusers: list of {email, name,
// role}) when the collection is still empty — the two planners for the first
// deployment. It never overwrites an existing (GUI-managed) allow-list. No-op
// without a DB or when auth.seedusers is unset.
func (p *Plexams) SeedUsers(ctx context.Context) {
	if p.dbClient == nil {
		return
	}
	existing, err := p.dbClient.GetUsers(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot read users for seeding")
		return
	}
	if len(existing) > 0 {
		return
	}
	var seed []struct {
		Email string `mapstructure:"email"`
		Name  string `mapstructure:"name"`
		Role  string `mapstructure:"role"`
	}
	if err := viper.UnmarshalKey("auth.seedusers", &seed); err != nil {
		log.Error().Err(err).Msg("cannot parse auth.seedusers")
		return
	}
	for _, s := range seed {
		email := strings.ToLower(strings.TrimSpace(s.Email))
		name := strings.TrimSpace(s.Name)
		if email == "" || name == "" {
			continue
		}
		role := model.Role(strings.ToUpper(strings.TrimSpace(s.Role)))
		if !role.IsValid() {
			role = model.RolePlaner
		}
		if err := p.dbClient.SaveUser(ctx, &model.User{Email: email, Name: name, Role: role}); err != nil {
			log.Error().Err(err).Str("email", email).Msg("cannot seed user")
			continue
		}
		log.Info().Str("email", email).Str("role", string(role)).Msg("seeded user")
	}
}

// LocalDevUser is the identity used when auth is disabled (local development), so
// local operation is unchanged: a full-access PLANER derived from auth.devuser, else
// the operator.* config, else a generic local identity. Never used when auth is on.
func (p *Plexams) LocalDevUser() *model.User {
	email := strings.ToLower(strings.TrimSpace(viper.GetString("auth.devuser")))
	name := ""
	if email == "" && p.operator != nil {
		email = strings.ToLower(strings.TrimSpace(p.operator.Email))
		name = strings.TrimSpace(p.operator.Name)
	}
	if email == "" {
		email = "local@localhost"
	}
	if name == "" {
		name = email
	}
	return &model.User{Email: email, Name: name, Role: model.RolePlaner}
}
