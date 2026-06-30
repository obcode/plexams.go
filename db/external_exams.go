package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
)

func (db *DB) AddExternalExam(ctx context.Context, exam *model.ZPAExam) error {
	_, err := db.Client.Database(db.databaseName).Collection(collectionExternalExams).InsertOne(ctx, exam)
	return err
}

// DeleteExternalExam removes a non-ZPA exam by its ancode.
func (db *DB) DeleteExternalExam(ctx context.Context, ancode int) error {
	_, err := db.Client.Database(db.databaseName).Collection(collectionExternalExams).
		DeleteOne(ctx, bson.M{"ancode": ancode})
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot delete non zpa exam")
	}
	return err
}

// RemovePlanEntry removes the plan entry (if any) of an ancode.
func (db *DB) RemovePlanEntry(ctx context.Context, ancode int) error {
	_, err := db.Client.Database(db.databaseName).Collection(collectionNamePlan).
		DeleteMany(ctx, bson.M{"ancode": ancode})
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot remove plan entry")
	}
	return err
}

func (db *DB) ExternalExam(ctx context.Context, ancode int) (*model.ZPAExam, error) {
	var exam model.ZPAExam
	err := db.Client.Database(db.databaseName).Collection(collectionExternalExams).FindOne(ctx, bson.M{"ancode": ancode}).Decode(&exam)
	if err != nil {
		log.Error().Err(err).Msg("cannot get non zpa exam")
		return nil, err
	}
	return &exam, nil
}

func (db *DB) ExternalExams(ctx context.Context) ([]*model.ZPAExam, error) {
	var exams []*model.ZPAExam
	cur, err := db.Client.Database(db.databaseName).Collection(collectionExternalExams).Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Msg("cannot get non zpa exams")
		return nil, err
	}
	defer cur.Close(ctx) //nolint:errcheck

	err = cur.All(ctx, &exams)
	if err != nil {
		log.Error().Err(err).Msg("cannot get non zpa exams")
		return nil, err
	}
	return exams, nil
}
