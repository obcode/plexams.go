package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
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
