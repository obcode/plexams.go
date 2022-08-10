package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
)

func (db *DB) GetZpaExamByAncode(ctx context.Context, anCode int) (*model.ZPAExam, error) {
	collection := db.Client.Database(databaseName(db.semester)).Collection("zpaexams")

	var result model.ZPAExam
	err := collection.FindOne(ctx, bson.D{{Key: "ancode", Value: anCode}}).Decode(&result)
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).
			Int("anCode", anCode).Msg("cannot find ZPA exam")
		return nil, err
	}

	return &result, nil
}
