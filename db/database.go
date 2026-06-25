package db

import (
	"context"
	"sort"
	"strings"

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
	return db.getSemesterConfigInputFrom(ctx, db.databaseName)
}

// GetSemesterConfigInputForSemester returns the raw config of another semester
// (its own database), or nil when none is stored. Used to seed a new semester
// from a previous one and to guard createSemester against overwriting.
func (db *DB) GetSemesterConfigInputForSemester(ctx context.Context, semester string) (*model.SemesterConfigInput, error) {
	return db.getSemesterConfigInputFrom(ctx, databaseNameForSemester(semester))
}

func (db *DB) getSemesterConfigInputFrom(ctx context.Context, databaseName string) (*model.SemesterConfigInput, error) {
	collection := db.Client.Database(databaseName).Collection(collectionNameSemesterConfigInput)
	var input model.SemesterConfigInput
	err := collection.FindOne(ctx, bson.M{}).Decode(&input)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		log.Error().Err(err).Str("database", databaseName).Msg("cannot get semester config input")
		return nil, err
	}
	return &input, nil
}

// SaveSemesterConfigInputForSemester writes the raw config into another
// semester's database (used when creating a new semester).
func (db *DB) SaveSemesterConfigInputForSemester(ctx context.Context, semester string, input *model.SemesterConfigInput) error {
	collection := db.Client.Database(databaseNameForSemester(semester)).Collection(collectionNameSemesterConfigInput)
	if err := collection.Drop(ctx); err != nil {
		return err
	}
	if _, err := collection.InsertOne(ctx, input); err != nil {
		log.Error().Err(err).Str("semester", semester).Msg("cannot save semester config input for semester")
		return err
	}
	return nil
}

// databaseNameForSemester maps a semester (e.g. "2026 WS" or "2026-WS") to its
// MongoDB database name ("2026-WS").
func databaseNameForSemester(semester string) string {
	return strings.Replace(semester, " ", "-", 1)
}

// MigrateLegacySemesterConfigInput rewrites a stored config that still carries the
// removed fromFK07 / dayNumberStart fields: `from` is set to the former numbering
// anchor (from when dayNumberStart == "from", else fromFK07) so existing plan day
// numbers stay stable, and the legacy fields are dropped. No-op otherwise.
func (db *DB) MigrateLegacySemesterConfigInput(ctx context.Context) error {
	collection := db.getCollectionSemester(collectionNameSemesterConfigInput)

	var doc bson.M
	err := collection.FindOne(ctx, bson.M{}).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil
		}
		return err
	}
	fromFK07, hasFK07 := doc["fromFK07"]
	_, hasDayNumberStart := doc["dayNumberStart"]
	if !hasFK07 && !hasDayNumberStart {
		return nil
	}

	set := bson.M{}
	if dns, _ := doc["dayNumberStart"].(string); dns != "from" && hasFK07 {
		set["from"] = fromFK07
	}
	update := bson.M{"$unset": bson.M{"fromFK07": "", "dayNumberStart": ""}}
	if len(set) > 0 {
		update["$set"] = set
	}
	if _, err := collection.UpdateOne(ctx, bson.M{"_id": doc["_id"]}, update); err != nil {
		log.Error().Err(err).Msg("cannot migrate legacy semester config input")
		return err
	}
	log.Info().Msg("migrated legacy semester config (removed fromFK07/dayNumberStart)")
	return nil
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
