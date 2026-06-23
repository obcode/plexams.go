package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// PermanentNonInvigilators returns the teachers who never do invigilation duty
// again. This list lives in the global "plexams" database and carries over
// between semesters.
func (db *DB) PermanentNonInvigilators(ctx context.Context) ([]*model.PermanentNonInvigilator, error) {
	collection := db.Client.Database("plexams").Collection(collectionPermanentNonInvigilators)

	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Msg("cannot find permanent non-invigilators")
		return nil, err
	}

	nonInvigilators := make([]*model.PermanentNonInvigilator, 0)
	if err := cur.All(ctx, &nonInvigilators); err != nil {
		log.Error().Err(err).Msg("cannot decode permanent non-invigilators")
		return nil, err
	}

	return nonInvigilators, nil
}

// UpsertPermanentNonInvigilator creates or replaces one permanent non-invigilator
// (key: teacherID).
func (db *DB) UpsertPermanentNonInvigilator(ctx context.Context, nonInvigilator *model.PermanentNonInvigilator) error {
	collection := db.Client.Database("plexams").Collection(collectionPermanentNonInvigilators)

	_, err := collection.ReplaceOne(ctx,
		bson.M{"teacherid": nonInvigilator.TeacherID},
		nonInvigilator,
		options.Replace().SetUpsert(true))
	if err != nil {
		log.Error().Err(err).Int("teacherID", nonInvigilator.TeacherID).Msg("cannot upsert permanent non-invigilator")
		return err
	}
	return nil
}

// DeletePermanentNonInvigilator removes one permanent non-invigilator. Returns
// false if there was none.
func (db *DB) DeletePermanentNonInvigilator(ctx context.Context, teacherID int) (bool, error) {
	collection := db.Client.Database("plexams").Collection(collectionPermanentNonInvigilators)

	res, err := collection.DeleteOne(ctx, bson.M{"teacherid": teacherID})
	if err != nil {
		log.Error().Err(err).Int("teacherID", teacherID).Msg("cannot delete permanent non-invigilator")
		return false, err
	}
	return res.DeletedCount > 0, nil
}
