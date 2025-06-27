package db

import (
	"context"

	"github.com/rs/zerolog/log"
)

func (db *DB) SaveStudentRegs(ctx context.Context, studentRegs []interface{}) error {
	collection := db.Client.Database(db.databaseName).Collection(collectionStudentRegsPerStudentPlanned)

	err := collection.Drop(ctx)
	if err != nil {
		log.Error().Err(err).Str("collectionName", collectionStudentRegsPerStudentPlanned).
			Msg("error while trying to drop the collection")
		return err
	}

	_, err = collection.InsertMany(ctx, studentRegs)
	if err != nil {
		log.Error().Err(err).Str("collectionName", collectionStudentRegsPerStudentPlanned).
			Msg("error while trying to insert")
		return err
	}

	return nil
}
