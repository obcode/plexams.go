package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
)

func (db *DB) GetZPAExams(ctx context.Context) ([]*model.ZPAExam, error) {
	collection := db.Client.Database(databaseName(db.semester)).Collection("zpaexams")

	exams := make([]*model.ZPAExam, 0)

	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("collection", "zpaexams").Msg("MongoDB Find")
		return exams, err
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var exam model.ZPAExam

		err := cur.Decode(&exam)
		if err != nil {
			log.Error().Err(err).Str("semester", db.semester).Str("collection", "zpaexams").Interface("cur", cur).
				Msg("Cannot decode to customer")
			return exams, err
		}

		exams = append(exams, &exam)
	}

	if err := cur.Err(); err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("collection", "zpaexams").Msg("Cursor returned error")
		return exams, err
	}

	return exams, nil
}
