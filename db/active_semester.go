package db

import (
	"context"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ActiveSemester records the last activated semester (and the database it lived in),
// stored globally so the next start can resume it.
type ActiveSemester struct {
	Semester string `bson:"semester"`
	Database string `bson:"database"`
}

// SaveActiveSemester remembers the current semester/database as the last active one.
func (db *DB) SaveActiveSemester(ctx context.Context) error {
	collection := db.Client.Database("plexams").Collection(collectionActiveSemester)
	_, err := collection.ReplaceOne(ctx, bson.M{},
		&ActiveSemester{Semester: db.semester, Database: db.databaseName},
		options.Replace().SetUpsert(true))
	if err != nil {
		log.Error().Err(err).Msg("cannot save active semester")
	}
	return err
}

// GetActiveSemester returns the last active semester, or nil when none is stored.
func (db *DB) GetActiveSemester(ctx context.Context) (*ActiveSemester, error) {
	collection := db.Client.Database("plexams").Collection(collectionActiveSemester)
	var active ActiveSemester
	err := collection.FindOne(ctx, bson.M{}).Decode(&active)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		log.Error().Err(err).Msg("cannot get active semester")
		return nil, err
	}
	return &active, nil
}

// DatabaseHasConfig reports whether a database carries a semester config (i.e. is
// usable with this code).
func (db *DB) DatabaseHasConfig(ctx context.Context, databaseName string) bool {
	config, err := db.getSemesterConfigInputFrom(ctx, databaseName)
	return err == nil && config != nil
}

// ResolveStartSemester picks the semester to start with when none is pinned: the
// last active one if it still has a config, otherwise the newest compatible
// semester. The returned database is empty when it should be derived from the
// semester. ok is false when nothing usable exists.
func (db *DB) ResolveStartSemester(ctx context.Context) (semester, database string, ok bool) {
	if active, _ := db.GetActiveSemester(ctx); active != nil && active.Database != "" {
		if db.DatabaseHasConfig(ctx, active.Database) {
			logical := db.metaSemesterOf(ctx, active.Database)
			if logical == "" {
				logical = active.Semester
			}
			if logical == "" {
				logical = semesterName(active.Database)
			}
			return logical, active.Database, true
		}
	}
	// AllSemesterNames is sorted newest first.
	sems, err := db.AllSemesterNames(ctx)
	if err == nil {
		for _, s := range sems {
			if s.Compatible {
				dbName := databaseNameForSemester(s.ID)
				logical := db.metaSemesterOf(ctx, dbName)
				if logical == "" {
					logical = s.ID
				}
				return logical, dbName, true
			}
		}
	}
	return "", "", false
}
