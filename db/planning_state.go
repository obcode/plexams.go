package db

import (
	"context"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// planningConditionDoc is one set planning condition (its presence = done).
type planningConditionDoc struct {
	Key string `bson:"key"`
}

// PlanningConditionsSet returns the keys of all conditions that are currently set
// (done) for the semester.
func (db *DB) PlanningConditionsSet(ctx context.Context) ([]string, error) {
	collection := db.getCollectionSemester(collectionPlanningState)
	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Msg("cannot get planning conditions")
		return nil, err
	}
	docs := make([]planningConditionDoc, 0)
	if err := cur.All(ctx, &docs); err != nil {
		log.Error().Err(err).Msg("cannot decode planning conditions")
		return nil, err
	}
	keys := make([]string, 0, len(docs))
	for _, d := range docs {
		keys = append(keys, d.Key)
	}
	return keys, nil
}

// SetPlanningCondition marks a condition as done (set=true, upsert) or removes it
// (set=false).
func (db *DB) SetPlanningCondition(ctx context.Context, key string, set bool) error {
	collection := db.getCollectionSemester(collectionPlanningState)
	if set {
		_, err := collection.ReplaceOne(ctx, bson.M{"key": key}, planningConditionDoc{Key: key},
			options.Replace().SetUpsert(true))
		if err != nil {
			log.Error().Err(err).Str("key", key).Msg("cannot set planning condition")
			return err
		}
		return nil
	}
	if _, err := collection.DeleteOne(ctx, bson.M{"key": key}); err != nil {
		log.Error().Err(err).Str("key", key).Msg("cannot unset planning condition")
		return err
	}
	return nil
}
