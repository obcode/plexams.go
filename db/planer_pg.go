package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// planerFromRow maps the stored overrides only. DefaultMail and the four
// Effective* fields of model.Planer are derived by plexams from these plus the
// config (see planerModel), and stay at their zero value here -- the same shape
// the Mongo document had, which holds nothing but name and email.
func planerFromRow(row sqlc.Planer) *model.Planer {
	return &model.Planer{
		Name:        row.Name,
		Email:       row.Email,
		TestMail:    row.TestMail,
		Cc:          row.Cc,
		NoreplyMail: row.NoreplyMail,
		NoreplyName: row.NoreplyName,
	}
}

// GetPlaner returns the planner (name/email plus the sender-identity overrides),
// or nil when none is stored.
func (db *PG) GetPlaner(ctx context.Context) (*model.Planer, error) {
	row, err := db.q(ctx).GetPlaner(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		log.Error().Err(err).Msg("cannot get planer")
		return nil, err
	}

	return planerFromRow(row), nil
}

// SavePlaner stores the planner. There is exactly one row, enforced by the
// check on the primary key rather than by an empty filter.
func (db *PG) SavePlaner(ctx context.Context, planer *model.Planer) error {
	err := db.q(ctx).SavePlaner(ctx, sqlc.SavePlanerParams{
		Name:        planer.Name,
		Email:       planer.Email,
		TestMail:    planer.TestMail,
		Cc:          planer.Cc,
		NoreplyMail: planer.NoreplyMail,
		NoreplyName: planer.NoreplyName,
	})
	if err != nil {
		log.Error().Err(err).Msg("cannot save planer")
		return err
	}
	return nil
}
