package db

import (
	"context"

	"github.com/rs/zerolog/log"
)

func collectionNameFromContext(ctx context.Context) string {
	if name, ok := ctx.Value(CollectionName("collectionName")).(string); ok {
		return name
	}
	return ""
}

func (db *DB) DropAndSave(ctx context.Context, objects []interface{}) error {
	collection := db.getCollectionSemesterFromContext(ctx)

	err := collection.Drop(ctx)
	if err != nil {
		log.Error().Err(err).
			Str("collectionName", collectionNameFromContext(ctx)).
			Msg("cannot drop collection")
		return err
	}

	if len(objects) == 0 {
		return nil
	}

	_, err = collection.InsertMany(ctx, objects)
	if err != nil {
		log.Error().Err(err).
			Str("collectionName", collectionNameFromContext(ctx)).
			Msg("cannot insert objects")
		return err
	}

	return nil
}

func (db *DB) Save(ctx context.Context, objects []interface{}) error {
	if len(objects) == 0 {
		return nil
	}

	collection := db.getCollectionSemesterFromContext(ctx)

	_, err := collection.InsertMany(ctx, objects)
	if err != nil {
		log.Error().Err(err).
			Str("collectionName", collectionNameFromContext(ctx)).
			Msg("cannot insert objects")
		return err
	}

	return nil
}
