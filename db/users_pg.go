package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// The app_user table is the login allow-list for the server deployment behind the
// auth proxy: the middleware looks a request's identity up here to authorize it,
// and absence means no access (fail-closed). It is global, carries over between
// semesters, and is kept strictly separate from the planer row (the shared email
// sender identity).
//
// The table is named app_user because "user" is a reserved word in SQL.

func userFromRow(row sqlc.AppUser) *model.User {
	return &model.User{
		Email:     row.Email,
		Name:      row.Name,
		Role:      model.Role(row.Role),
		Shortname: row.Shortname,
	}
}

// GetUsers returns all known users (the allow-list), sorted by email.
func (db *PG) GetUsers(ctx context.Context) ([]*model.User, error) {
	rows, err := db.q(ctx).ListUsers(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get users")
		return nil, err
	}

	users := make([]*model.User, 0, len(rows))
	for _, row := range rows {
		users = append(users, userFromRow(row))
	}
	return users, nil
}

// GetUserByEmail returns the user with the given email, or nil when none is stored.
func (db *PG) GetUserByEmail(ctx context.Context, email string) (*model.User, error) {
	row, err := db.q(ctx).GetUser(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		log.Error().Err(err).Str("email", email).Msg("cannot get user")
		return nil, err
	}

	return userFromRow(row), nil
}

// SaveUser upserts a user keyed by email.
func (db *PG) SaveUser(ctx context.Context, user *model.User) error {
	err := db.q(ctx).SaveUser(ctx, sqlc.SaveUserParams{
		Email:     user.Email,
		Name:      user.Name,
		Role:      string(user.Role),
		Shortname: user.Shortname,
	})
	if err != nil {
		log.Error().Err(err).Str("email", user.Email).Msg("cannot save user")
		return err
	}
	return nil
}

// DeleteUser removes the user with the given email.
func (db *PG) DeleteUser(ctx context.Context, email string) error {
	if err := db.q(ctx).DeleteUser(ctx, email); err != nil {
		log.Error().Err(err).Str("email", email).Msg("cannot delete user")
		return err
	}
	return nil
}
