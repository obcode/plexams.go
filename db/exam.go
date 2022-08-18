package db

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func (db *DB) AddExam(ctx context.Context, exam *model.Exam) error {
	collection := db.Client.Database(databaseName(db.semester)).Collection("exams")

	result := collection.FindOne(ctx, bson.D{{Key: "anCode", Value: exam.AnCode}})
	if result.Err() == nil {
		log.Error().Int("anCode", exam.AnCode).Msg("cannot add exam, exam with ancode already in db")
		return fmt.Errorf("cannot add exam, exam with ancode %d already in db", exam.AnCode)
	}

	if result.Err() == mongo.ErrNoDocuments {
		_, err := collection.InsertOne(ctx, exam)
		if err != nil {
			return err
		}
		return nil
	}

	return result.Err()
}
