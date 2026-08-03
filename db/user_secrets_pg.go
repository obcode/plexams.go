package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/obcode/plexams.go/plexams/secrets"
	"github.com/rs/zerolog/log"
)

// userSecretFromRow reassembles the sealed value from its three columns. The
// schema check keeps them all-or-nothing (a half-written secret cannot be
// opened), so testing one of them would be enough -- all three are checked
// anyway, because the alternative is a nil dereference on a row that somehow got
// past the constraint.
func userSecretFromRow(row sqlc.UserSecret) *UserSecret {
	s := &UserSecret{
		Email:         row.Email,
		JiraUpdatedAt: row.JiraUpdatedAt,
	}
	if row.JiraKeyVersion != nil && row.JiraNonce != nil && row.JiraCiphertext != nil {
		s.Jira = &secrets.SealedValue{
			KeyVersion: *row.JiraKeyVersion,
			Nonce:      row.JiraNonce,
			Ciphertext: row.JiraCiphertext,
		}
	}
	return s
}

// GetUserSecret returns the stored secrets for a user, or nil when none exist.
func (db *PG) GetUserSecret(ctx context.Context, email string) (*UserSecret, error) {
	row, err := db.q(ctx).GetUserSecret(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		log.Error().Err(err).Str("email", email).Msg("cannot get user secret")
		return nil, err
	}

	return userSecretFromRow(row), nil
}

// SaveUserJiraToken upserts the sealed Jira PAT for a user.
func (db *PG) SaveUserJiraToken(ctx context.Context, email string, sealed secrets.SealedValue, updatedAt time.Time) error {
	err := db.q(ctx).SaveUserJiraToken(ctx, sqlc.SaveUserJiraTokenParams{
		Email:          email,
		JiraKeyVersion: &sealed.KeyVersion,
		JiraNonce:      sealed.Nonce,
		JiraCiphertext: sealed.Ciphertext,
		JiraUpdatedAt:  &updatedAt,
	})
	if err != nil {
		log.Error().Err(err).Str("email", email).Msg("cannot save user jira token")
	}
	return err
}

// DeleteUserJiraToken removes only the Jira PAT from a user's secrets, leaving
// the row for whatever else it may hold later.
func (db *PG) DeleteUserJiraToken(ctx context.Context, email string) error {
	if err := db.q(ctx).DeleteUserJiraToken(ctx, email); err != nil {
		log.Error().Err(err).Str("email", email).Msg("cannot delete user jira token")
		return err
	}
	return nil
}
