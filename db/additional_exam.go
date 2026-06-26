package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// AdditionalExams returns all additional (publish-only) exams.
func (db *DB) AdditionalExams(ctx context.Context) ([]*model.AdditionalExam, error) {
	collection := db.getCollectionSemester(collectionAdditionalExams)
	cur, err := collection.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "ancode", Value: 1}}))
	if err != nil {
		log.Error().Err(err).Msg("cannot find additional exams")
		return nil, err
	}
	exams := make([]*model.AdditionalExam, 0)
	if err := cur.All(ctx, &exams); err != nil {
		log.Error().Err(err).Msg("cannot decode additional exams")
		return nil, err
	}
	return exams, nil
}

// UpsertAdditionalExam creates or updates one additional exam (key: ancode).
func (db *DB) UpsertAdditionalExam(ctx context.Context, exam *model.AdditionalExam) error {
	collection := db.getCollectionSemester(collectionAdditionalExams)
	_, err := collection.ReplaceOne(ctx, bson.M{"ancode": exam.Ancode}, exam, options.Replace().SetUpsert(true))
	if err != nil {
		log.Error().Err(err).Int("ancode", exam.Ancode).Msg("cannot upsert additional exam")
		return err
	}
	return nil
}

// DeleteAdditionalExam removes one additional exam by ancode.
func (db *DB) DeleteAdditionalExam(ctx context.Context, ancode int) (bool, error) {
	collection := db.getCollectionSemester(collectionAdditionalExams)
	res, err := collection.DeleteOne(ctx, bson.M{"ancode": ancode})
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot delete additional exam")
		return false, err
	}
	return res.DeletedCount > 0, nil
}
