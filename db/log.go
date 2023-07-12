package db

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
)

func (db *DB) Log(ctx context.Context, subj, msg string) error {
	collection := db.Client.Database(db.databaseName).Collection("log")

	doc := bson.D{
		{Key: "created", Value: time.Now()},
		{Key: "subj", Value: subj},
	}

	if len(msg) > 0 {
		doc = append(doc, bson.E{Key: "msg", Value: msg})
	}

	_, err := collection.InsertOne(ctx, doc)
	if err != nil {
		log.Error().Err(err).Str("msg", msg).Msg("cannot log message in MongoDB")
		return err
	}

	return nil
}
