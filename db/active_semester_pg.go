package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rs/zerolog/log"
)

// SaveActiveSemester remembers the current semester as the last active one.
//
// New against Mongo: the column references semester(id), so a semester that is
// not in the registry cannot be recorded as active. That is the point -- the
// Mongo document could name a database that had since been dropped, and the next
// start would try to resume into it.
//
// The rejection is reported as ErrSemesterNotRegistered rather than logged as an
// error: it is what every first boot against an empty database does, because the
// semester is pinned with --semester before createSemester has ever run.
func (db *PG) SaveActiveSemester(ctx context.Context) error {
	if err := db.q(ctx).SaveActiveSemester(ctx, db.semesterID); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgErrForeignKeyViolation {
			log.Debug().Str("semesterID", db.semesterID).
				Msg("not remembering an active semester that is not registered yet")
			return fmt.Errorf("%w: %s", ErrSemesterNotRegistered, db.semesterID)
		}
		log.Error().Err(err).Str("semesterID", db.semesterID).Msg("cannot save active semester")
		return err
	}
	return nil
}

// GetActiveSemester returns the last active semester, or nil when none is stored.
//
// One value where the Mongo document carried the database name AND a logical
// semester next to it, which could disagree.
func (db *PG) GetActiveSemester(ctx context.Context) (*ActiveSemester, error) {
	semester, err := db.q(ctx).GetActiveSemester(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		log.Error().Err(err).Msg("cannot get active semester")
		return nil, err
	}

	return &ActiveSemester{Semester: semester}, nil
}
