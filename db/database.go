package db

import (
	"context"
	"sort"
	"strings"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// systemDatabases are never plexams workspaces.
var systemDatabases = map[string]bool{"admin": true, "local": true, "config": true, "plexams": true}

// DatabaseName returns the name of the database the client is currently pointed at.
func (db *DB) DatabaseName() string {
	return db.databaseName
}

// AllSemesterNames lists the plexams workspaces: every database carrying a semester
// config or meta. The id is the database name (the switch key); semester is the
// logical semester (for ZPA), which may differ from the database name.
func (db *DB) AllSemesterNames(ctx context.Context) ([]*model.Semester, error) {
	dbs, err := db.Client.ListDatabaseNames(ctx, bson.M{})
	if err != nil {
		return nil, err
	}

	semester := make([]*model.Semester, 0, len(dbs))
	for _, dbName := range dbs {
		if systemDatabases[dbName] {
			continue
		}
		config, _ := db.getSemesterConfigInputFrom(ctx, dbName)
		meta, _ := db.getSemesterMetaFrom(ctx, dbName)
		if config == nil && meta == nil {
			continue // not a plexams workspace
		}
		sem := &model.Semester{
			ID:         dbName,
			Compatible: config != nil,
		}
		if meta != nil {
			sem.ReadOnly = meta.ReadOnly
			v := meta.SchemaVersion
			sem.SchemaVersion = &v
			if meta.Semester != "" {
				s := meta.Semester
				sem.Semester = &s
			}
		}
		semester = append(semester, sem)
	}

	// newest first: by logical semester descending (e.g. 2026 WS > 2026 SS > 2025 WS),
	// then the canonical database before test workspaces of the same semester.
	logicalOf := func(s *model.Semester) string {
		if s.Semester != nil {
			return *s.Semester
		}
		return semesterName(s.ID)
	}
	sort.Slice(semester, func(i, j int) bool {
		li, lj := logicalOf(semester[i]), logicalOf(semester[j])
		if li != lj {
			return li > lj
		}
		return semester[i].ID < semester[j].ID
	})

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
	return db.SaveSemesterConfigInputToDatabase(ctx, databaseNameForSemester(semester), input)
}

// SaveSemesterConfigInputToDatabase writes the raw config into a specific database
// (by exact name; used when creating a workspace with an arbitrary database name).
func (db *DB) SaveSemesterConfigInputToDatabase(ctx context.Context, database string, input *model.SemesterConfigInput) error {
	collection := db.Client.Database(database).Collection(collectionNameSemesterConfigInput)
	if err := collection.Drop(ctx); err != nil {
		return err
	}
	if _, err := collection.InsertOne(ctx, input); err != nil {
		log.Error().Err(err).Str("database", database).Msg("cannot save semester config input")
		return err
	}
	return nil
}

// SemesterConfigInputForDatabase returns the raw config stored in a specific
// database (by exact name), or nil when none.
func (db *DB) SemesterConfigInputForDatabase(ctx context.Context, database string) (*model.SemesterConfigInput, error) {
	return db.getSemesterConfigInputFrom(ctx, database)
}

// databaseNameForSemester maps a semester (e.g. "2026 WS" or "2026-WS") to its
// MongoDB database name ("2026-WS").
func databaseNameForSemester(semester string) string {
	return strings.Replace(semester, " ", "-", 1)
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
