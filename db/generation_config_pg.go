package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// generationConfigFormatVersion is the shape of model.GenerationConfig this
// binary reads and writes. Bump it when a field is renamed or its meaning
// changes -- not when one is added, which json handles by itself.
//
// The point of the version is that the json tags of model.GenerationConfig are
// simultaneously the GraphQL contract: a rename in generation_config.graphqls
// changes the storage format without touching anything that looks like storage.
// Reading a blob written by a newer binary has to fail loudly, because the
// alternative is 33 weights silently falling back to their zero value -- which
// the generator would happily run with.
const generationConfigFormatVersion = 1

// GetGenerationConfig returns the (single, global) generation config, or nil when
// none is stored yet.
func (db *PG) GetGenerationConfig(ctx context.Context) (*model.GenerationConfig, error) {
	row, err := db.q(ctx).GetGenerationConfig(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		log.Error().Err(err).Msg("cannot get generation config")
		return nil, err
	}

	if row.FormatVersion != generationConfigFormatVersion {
		err := fmt.Errorf("generation config was written in format version %d, this binary reads %d",
			row.FormatVersion, generationConfigFormatVersion)
		log.Error().Err(err).Msg("cannot get generation config")
		return nil, err
	}

	var cfg model.GenerationConfig
	if err := json.Unmarshal(row.Config, &cfg); err != nil {
		log.Error().Err(err).Msg("cannot decode generation config")
		return nil, err
	}

	return &cfg, nil
}

// SetGenerationConfig upserts the (single, global) generation config.
func (db *PG) SetGenerationConfig(ctx context.Context, cfg *model.GenerationConfig) error {
	blob, err := json.Marshal(cfg)
	if err != nil {
		log.Error().Err(err).Msg("cannot encode generation config")
		return err
	}

	err = db.q(ctx).SetGenerationConfig(ctx, sqlc.SetGenerationConfigParams{
		Config:        blob,
		FormatVersion: generationConfigFormatVersion,
	})
	if err != nil {
		log.Error().Err(err).Msg("cannot set generation config")
		return err
	}
	return nil
}
