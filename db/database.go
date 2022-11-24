package db

import (
	"context"
	"sort"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var collectionNameSemesterConfig = "semester_config"

func (db *DB) AllSemesterNames() ([]*model.Semester, error) {
	dbs, err := db.Client.ListDatabaseNames(context.Background(),
		bson.D{primitive.E{
			Key: "name",
			Value: bson.D{
				primitive.E{Key: "$regex",
					Value: primitive.Regex{Pattern: "[0-9]{4}-[WS]S"},
				},
			},
		}})
	if err != nil {
		return nil, err
	}

	sort.Strings(dbs)

	semester := make([]*model.Semester, len(dbs))
	n := len(dbs)
	for i, dbName := range dbs {
		semester[n-i-1] = &model.Semester{
			ID: semesterName(dbName),
		}
	}

	return semester, nil
}

func (db *DB) SaveSemesterConfig(ctx context.Context, semesterConfig *model.SemesterConfig) error {
	collection := db.Client.Database(databaseName(db.semester)).Collection(collectionNameSemesterConfig)

	err := collection.Drop(ctx)
	if err != nil {
		return err
	}

	_, err = collection.InsertOne(ctx, semesterConfig)
	if err != nil {
		log.Error().Err(err).Msg("cannot save semester config")
		return err
	}

	return nil
}
