package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// GetGenerationConfig returns the (single, global) generation config, or nil when
// none is stored yet.
func (db *DB) GetGenerationConfig(ctx context.Context) (*model.GenerationConfig, error) {
	collection := db.Client.Database("plexams").Collection(collectionGenerationConfig)
	var cfg model.GenerationConfig
	err := collection.FindOne(ctx, bson.M{}).Decode(&cfg)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		log.Error().Err(err).Msg("cannot get generation config")
		return nil, err
	}
	return &cfg, nil
}

// SetGenerationConfig upserts the (single, global) generation config.
func (db *DB) SetGenerationConfig(ctx context.Context, cfg *model.GenerationConfig) error {
	collection := db.Client.Database("plexams").Collection(collectionGenerationConfig)
	_, err := collection.ReplaceOne(ctx, bson.M{}, cfg, options.Replace().SetUpsert(true))
	if err != nil {
		log.Error().Err(err).Msg("cannot set generation config")
		return err
	}
	return nil
}
