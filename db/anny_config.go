package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// GetAnnyConfig returns the global Anny config, or nil when none is stored yet.
func (db *DB) GetAnnyConfig(ctx context.Context) (*model.AnnyConfig, error) {
	collection := db.Client.Database("plexams").Collection(collectionAnnyConfig)
	var cfg model.AnnyConfig
	err := collection.FindOne(ctx, bson.M{}).Decode(&cfg)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		log.Error().Err(err).Msg("cannot get anny config")
		return nil, err
	}
	return &cfg, nil
}

// SetAnnyConfig upserts the (single, global) Anny config.
func (db *DB) SetAnnyConfig(ctx context.Context, cfg *model.AnnyConfig) error {
	collection := db.Client.Database("plexams").Collection(collectionAnnyConfig)
	_, err := collection.ReplaceOne(ctx, bson.M{}, cfg, options.Replace().SetUpsert(true))
	if err != nil {
		log.Error().Err(err).Msg("cannot set anny config")
		return err
	}
	return nil
}
