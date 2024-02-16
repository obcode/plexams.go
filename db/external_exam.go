package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
)

func (db *DB) AddExternalExam(ctx context.Context, exam *model.ExternalExam) error {
	collection := db.Client.Database(db.databaseName).Collection(collectionNameExternalExams)
	_, err := collection.InsertOne(ctx, exam)
	if err != nil {
		log.Error().Err(err).Interface("exam", exam).
			Msg("cannot insert external exam to database")
	}

	return err
}

func (db *DB) ExternalExams(ctx context.Context) ([]*model.ExternalExam, error) {
	collection := db.Client.Database(db.databaseName).Collection(collectionNameExternalExams)

	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Msg("cannot get external exams")
		return nil, err
	}
	defer cur.Close(ctx)

	var result []*model.ExternalExam
	err = cur.All(ctx, &result)
	if err != nil {
		log.Error().Err(err).Msg("cannot decode external exams")
		return nil, err
	}

	return result, nil
}
