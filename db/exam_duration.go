package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// SetExamDurationOverride upserts the duration override (minutes) for an ancode.
func (db *DB) SetExamDurationOverride(ctx context.Context, ancode, duration int) (*model.ExamDurationOverride, error) {
	collection := db.getCollectionSemester(collectionExamDurationOverride)
	override := &model.ExamDurationOverride{Ancode: ancode, Duration: duration}
	_, err := collection.ReplaceOne(ctx, bson.M{"ancode": ancode}, override, options.Replace().SetUpsert(true))
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot set exam duration override")
		return nil, err
	}
	return override, nil
}

// RemoveExamDurationOverride deletes the duration override for an ancode; returns
// false when there was none.
func (db *DB) RemoveExamDurationOverride(ctx context.Context, ancode int) (bool, error) {
	collection := db.getCollectionSemester(collectionExamDurationOverride)
	res, err := collection.DeleteOne(ctx, bson.M{"ancode": ancode})
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot remove exam duration override")
		return false, err
	}
	return res.DeletedCount > 0, nil
}

// ExamDurationOverrides returns all duration overrides.
func (db *DB) ExamDurationOverrides(ctx context.Context) ([]*model.ExamDurationOverride, error) {
	collection := db.getCollectionSemester(collectionExamDurationOverride)
	cur, err := collection.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "ancode", Value: 1}}))
	if err != nil {
		log.Error().Err(err).Msg("cannot find exam duration overrides")
		return nil, err
	}
	overrides := make([]*model.ExamDurationOverride, 0)
	if err := cur.All(ctx, &overrides); err != nil {
		log.Error().Err(err).Msg("cannot decode exam duration overrides")
		return nil, err
	}
	return overrides, nil
}
