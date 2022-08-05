package db

import (
	"context"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (db *DB) CacheTeachers(teachers []*model.Teacher, semester string) error {
	collection := db.Client.Database(databaseName(semester)).Collection("teachers")

	teachersIntf := make([]interface{}, 0, len(teachers))

	for _, v := range teachers {
		teachersIntf = append(teachersIntf, v)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := collection.Drop(ctx)
	if err != nil {
		return err
	}

	res, err := collection.InsertMany(ctx, teachersIntf)
	if err != nil {
		return err
	}

	log.Debug().Str("semester", semester).Int("documents", len(res.InsertedIDs)).Msg("inserted teachers")

	return nil
}
