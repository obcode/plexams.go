package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// GetAnnyConfig returns the global Anny config, or nil when none is stored yet.
func (db *PG) GetAnnyConfig(ctx context.Context) (*model.AnnyConfig, error) {
	row, err := db.q(ctx).GetAnnyConfig(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		log.Error().Err(err).Msg("cannot get anny config")
		return nil, err
	}

	names := row.PersonalizationNames
	if names == nil {
		// personalizationNames is a GraphQL non-null list; a nil slice would
		// serialise to null and the GUI would render nothing rather than "none".
		names = make([]string, 0)
	}

	return &model.AnnyConfig{PersonalizationNames: names}, nil
}

// SetAnnyConfig upserts the (single, global) Anny config.
func (db *PG) SetAnnyConfig(ctx context.Context, cfg *model.AnnyConfig) error {
	names := cfg.PersonalizationNames
	if names == nil {
		names = make([]string, 0)
	}

	if err := db.q(ctx).SetAnnyConfig(ctx, names); err != nil {
		log.Error().Err(err).Msg("cannot set anny config")
		return err
	}
	return nil
}
