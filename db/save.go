package db

import (
	"context"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
)

// ReplaceTarget names a collection that may be replaced wholesale. It exists so callers
// outside this package can pick a target without naming a collection: the name used to
// travel as an untyped string through context.Value, where a typo silently wrote into a
// different (newly created) collection and a missing value panicked on the type
// assertion.
type ReplaceTarget string

const (
	TargetZPAStudents             ReplaceTarget = collectionZpaStudents
	TargetInvigilatorRequirements ReplaceTarget = collectionInvigilatorRequirements
	TargetSelfInvigilations       ReplaceTarget = collectionSelfInvigilations
	TargetOtherInvigilations      ReplaceTarget = collectionOtherInvigilations
)

// ReplaceAll swaps the entire contents of one collection for objects, atomically where
// the deployment allows it (see withTransaction). An empty slice clears the collection.
//
// It clears with DeleteMany rather than dropping: dropping is not permitted inside a
// transaction, and it would also discard any indexes on the collection.
func (db *DB) ReplaceAll(ctx context.Context, target ReplaceTarget, objects []interface{}) error {
	return db.withTransaction(ctx, func(ctx context.Context) error {
		collection := db.getCollectionSemester(string(target))

		if _, err := collection.DeleteMany(ctx, bson.M{}); err != nil {
			log.Error().Err(err).Str("collection", string(target)).Msg("cannot clear collection")
			return err
		}

		if len(objects) == 0 {
			return nil
		}

		if _, err := collection.InsertMany(ctx, objects); err != nil {
			log.Error().Err(err).Str("collection", string(target)).Msg("cannot insert objects")
			return err
		}
		return nil
	})
}
