package db

import (
	"context"

	"github.com/rs/zerolog/log"
)

func (db *DB) Save(ctx context.Context, objects []interface{}) error {
	collection := db.getCollectionSemester(ctx)

	_, err := collection.InsertMany(ctx, objects)
	if err != nil {
		log.Error().Err(err).
			Str("collectionName", ctx.Value("collectionName").(string)).
			Msg("cannot insert objects")
		return err
	}

	return nil
}
