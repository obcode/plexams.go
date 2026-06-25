package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// GetPlaner returns the planner (name/email), or nil when none is stored. The
// planner lives in the global "plexams" database (not a secret, carries over
// between semesters).
func (db *DB) GetPlaner(ctx context.Context) (*model.Planer, error) {
	collection := db.Client.Database("plexams").Collection(collectionPlaner)
	var planer model.Planer
	err := collection.FindOne(ctx, bson.M{}).Decode(&planer)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		log.Error().Err(err).Msg("cannot get planer")
		return nil, err
	}
	return &planer, nil
}

// SavePlaner stores the planner (single document, replaced on upsert).
func (db *DB) SavePlaner(ctx context.Context, planer *model.Planer) error {
	collection := db.Client.Database("plexams").Collection(collectionPlaner)
	if _, err := collection.ReplaceOne(ctx, bson.M{}, planer, options.Replace().SetUpsert(true)); err != nil {
		log.Error().Err(err).Msg("cannot save planer")
		return err
	}
	return nil
}
