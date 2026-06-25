package db

import (
	"context"
	"sort"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

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

// GetSemesterConfigInput returns the raw, editable per-semester config (the
// source of truth) or nil when none has been stored yet.
func (db *DB) GetSemesterConfigInput(ctx context.Context) (*model.SemesterConfigInput, error) {
	collection := db.getCollectionSemester(collectionNameSemesterConfigInput)
	var input model.SemesterConfigInput
	err := collection.FindOne(ctx, bson.M{}).Decode(&input)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		log.Error().Err(err).Msg("cannot get semester config input")
		return nil, err
	}
	return &input, nil
}

// SaveSemesterConfigInput replaces the stored raw per-semester config.
func (db *DB) SaveSemesterConfigInput(ctx context.Context, input *model.SemesterConfigInput) error {
	collection := db.getCollectionSemester(collectionNameSemesterConfigInput)

	if err := collection.Drop(ctx); err != nil {
		return err
	}
	if _, err := collection.InsertOne(ctx, input); err != nil {
		log.Error().Err(err).Msg("cannot save semester config input")
		return err
	}
	return nil
}

func (db *DB) SaveSemesterConfig(ctx context.Context, semesterConfig *model.SemesterConfig) error {
	collection := db.Client.Database(db.databaseName).Collection(collectionNameSemesterConfig)

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
