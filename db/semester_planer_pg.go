package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/rs/zerolog/log"
)

// GetSemesterPlaner returns the current semester's planner override, or nil when
// the semester has none and inherits the planner from the server config.
//
// Only the overrides are stored. The resolved planner -- and with it
// model.Planer's DefaultMail and the four Effective* fields -- is derived in
// plexams (see resolvePlaner/planerModel), so no derived value can outlive the
// override it came from.
func (db *PG) GetSemesterPlaner(ctx context.Context) (*SemesterPlaner, error) {
	row, err := db.q(ctx).GetSemesterPlaner(ctx, db.semesterID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		log.Error().Err(err).Str("semesterID", db.semesterID).Msg("cannot get semester planer")
		return nil, err
	}

	return &SemesterPlaner{
		Name:        row.Name,
		Email:       row.Email,
		TestMail:    row.TestMail,
		Cc:          row.Cc,
		NoreplyMail: row.NoreplyMail,
		NoreplyName: row.NoreplyName,
	}, nil
}

// SaveSemesterPlaner stores the current semester's planner override, replacing
// any previous one. nil fields are stored as NULL, i.e. as "inherit".
//
// The semester has to be registered: the foreign key rejects an override for one
// that is not.
func (db *PG) SaveSemesterPlaner(ctx context.Context, planer *SemesterPlaner) error {
	err := db.q(ctx).SaveSemesterPlaner(ctx, sqlc.SaveSemesterPlanerParams{
		SemesterID:  db.semesterID,
		Name:        planer.Name,
		Email:       planer.Email,
		TestMail:    planer.TestMail,
		Cc:          planer.Cc,
		NoreplyMail: planer.NoreplyMail,
		NoreplyName: planer.NoreplyName,
	})
	if err != nil {
		log.Error().Err(err).Str("semesterID", db.semesterID).Msg("cannot save semester planer")
	}
	return err
}

// DeleteSemesterPlaner drops the current semester's override, so the semester
// inherits the planner from the server config again. Deleting a row that is not
// there is not an error.
func (db *PG) DeleteSemesterPlaner(ctx context.Context) error {
	err := db.q(ctx).DeleteSemesterPlaner(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Str("semesterID", db.semesterID).Msg("cannot delete semester planer")
	}
	return err
}
