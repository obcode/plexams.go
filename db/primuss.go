package db

import (
	"context"
	"strings"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func (db *DB) GetPrimussExams(ctx context.Context) ([]*model.PrimussExamByGroup, error) {
	collections, err := db.Client.Database(databaseName(db.semester)).ListCollectionNames(ctx,
		bson.D{primitive.E{
			Key: "name",
			Value: bson.D{
				primitive.E{Key: "$regex",
					Value: primitive.Regex{Pattern: "exams_"},
				},
			},
		}})

	primussExams := make([]*model.PrimussExamByGroup, 0)
	for _, collectionName := range collections {
		exams, err := db.getPrimussExams(ctx, collectionName)
		if err != nil {
			return primussExams, err
		}
		primussExams = append(primussExams, &model.PrimussExamByGroup{
			Group: strings.Replace(collectionName, "exams_", "", 1),
			Exams: exams,
		})
	}

	return primussExams, err
}

func (db *DB) getPrimussExams(ctx context.Context, collectionName string) ([]*model.PrimussExam, error) {
	collection := db.Client.Database(databaseName(db.semester)).Collection(collectionName)

	exams := make([]*model.PrimussExam, 0)

	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("collection", collectionName).Msg("MongoDB Find")
		return exams, err
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var exam model.PrimussExam

		err := cur.Decode(&exam)
		if err != nil {
			log.Error().Err(err).Str("semester", db.semester).Str("collection", collectionName).Interface("cur", cur).
				Msg("Cannot decode to exam")
			return exams, err
		}

		exams = append(exams, &exam)
	}

	if err := cur.Err(); err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("collection", collectionName).Msg("Cursor returned error")
		return exams, err
	}

	return exams, nil
}
