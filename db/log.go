package db

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
)

func (db *DB) Log(ctx context.Context, msg string) error {
	collection := db.Client.Database(databaseName(db.semester)).Collection("log")

	_, err := collection.InsertOne(ctx, bson.D{
		{Key: "created", Value: time.Now()},
		{Key: "msg", Value: msg},
	})
	if err != nil {
		log.Error().Err(err).Str("msg", msg).Msg("cannot log message in MongoDB")
		return err
	}

	return nil
}
