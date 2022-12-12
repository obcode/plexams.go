package db

import (
	"context"

	"github.com/rs/zerolog/log"
)

func (db *DB) DropAndSave(ctx context.Context, objects []interface{}) error {
	collection := db.getCollectionSemesterFromContext(ctx)

	err := collection.Drop(ctx)
	if err != nil {
		log.Error().Err(err).
			Str("collectionName", ctx.Value("collectionName").(string)).
			Msg("cannot drop collection")
		return err
	}

	_, err = collection.InsertMany(ctx, objects)
	if err != nil {
		log.Error().Err(err).
			Str("collectionName", ctx.Value("collectionName").(string)).
			Msg("cannot insert objects")
		return err
	}

	return nil
}

func (db *DB) Save(ctx context.Context, objects []interface{}) error {
	collection := db.getCollectionSemesterFromContext(ctx)

	_, err := collection.InsertMany(ctx, objects)
	if err != nil {
		log.Error().Err(err).
			Str("collectionName", ctx.Value("collectionName").(string)).
			Msg("cannot insert objects")
		return err
	}

	return nil
}
