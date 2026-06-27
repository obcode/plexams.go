package db

import (
	"context"
	"strings"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// SemesterMeta is per-database metadata (not planning config): the data schema
// version (compatibility), the read-only flag and the logical semester (used
// against external systems like ZPA, so a clone keeps the real semester instead of
// its database name).
type SemesterMeta struct {
	SchemaVersion int    `bson:"schemaVersion"`
	ReadOnly      bool   `bson:"readOnly"`
	Semester      string `bson:"semester,omitempty"`
}

func (db *DB) getSemesterMetaFrom(ctx context.Context, databaseName string) (*SemesterMeta, error) {
	collection := db.Client.Database(databaseName).Collection(collectionSemesterMeta)
	var meta SemesterMeta
	err := collection.FindOne(ctx, bson.M{}).Decode(&meta)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		log.Error().Err(err).Str("database", databaseName).Msg("cannot get semester meta")
		return nil, err
	}
	return &meta, nil
}

// GetSemesterMeta returns the meta of the current database (nil when none).
func (db *DB) GetSemesterMeta(ctx context.Context) (*SemesterMeta, error) {
	return db.getSemesterMetaFrom(ctx, db.databaseName)
}

// EnsureMeta stamps the current database's schema version if it has none yet,
// preserving the read-only flag and semester. It does NOT write the semester, since
// a derived/guessed semester must not be persisted (only explicit ones are, via
// SetMetaSemester).
func (db *DB) EnsureMeta(ctx context.Context, version int) error {
	meta, err := db.GetSemesterMeta(ctx)
	if err != nil {
		return err
	}
	collection := db.Client.Database(db.databaseName).Collection(collectionSemesterMeta)
	if meta == nil {
		if _, err := collection.InsertOne(ctx, &SemesterMeta{SchemaVersion: version, ReadOnly: false}); err != nil {
			log.Error().Err(err).Msg("cannot stamp schema version")
			return err
		}
		return nil
	}
	if meta.SchemaVersion == 0 {
		if _, err := collection.UpdateOne(ctx, bson.M{}, bson.M{"$set": bson.M{"schemaVersion": version}}); err != nil {
			log.Error().Err(err).Msg("cannot update schema version")
			return err
		}
	}
	return nil
}

// SetMetaSemester force-writes the logical semester of the current database
// (authoritative: an explicit pin/override/createSemester). Overwrites any previous
// value; creates the meta with the given schema version if missing.
func (db *DB) SetMetaSemester(ctx context.Context, semester string, version int) error {
	return db.setMetaSemesterIn(ctx, db.databaseName, semester, version)
}

// SetMetaSemesterForSemester force-writes the logical semester into another
// semester's database (used when creating a new semester).
func (db *DB) SetMetaSemesterForSemester(ctx context.Context, semester string, version int) error {
	return db.setMetaSemesterIn(ctx, databaseNameForSemester(semester), semester, version)
}

func (db *DB) setMetaSemesterIn(ctx context.Context, databaseName, semester string, version int) error {
	collection := db.Client.Database(databaseName).Collection(collectionSemesterMeta)
	_, err := collection.UpdateOne(ctx, bson.M{},
		bson.M{
			"$set":         bson.M{"semester": semester},
			"$setOnInsert": bson.M{"schemaVersion": version, "readOnly": false},
		},
		options.Update().SetUpsert(true))
	if err != nil {
		log.Error().Err(err).Str("database", databaseName).Msg("cannot set meta semester")
	}
	return err
}

// metaSemesterOf returns the stored logical semester of a database, or "" when none.
func (db *DB) metaSemesterOf(ctx context.Context, databaseName string) string {
	if meta, _ := db.getSemesterMetaFrom(ctx, databaseName); meta != nil {
		return meta.Semester
	}
	return ""
}

// SwitchTo repoints the DB to the database identified by id (an allSemesterNames
// label, e.g. "2026 SS" or a clone "2026 SS-Test"). The logical semester is the
// override when given, else the database's own stored semester, else derived from
// the id. Returns the resolved logical semester.
func (db *DB) SwitchTo(ctx context.Context, id, override string) string {
	dbName := databaseNameForSemester(id)
	logical := strings.TrimSpace(override)
	if logical == "" {
		logical = db.metaSemesterOf(ctx, dbName)
	}
	if logical == "" {
		logical = semesterName(dbName)
	}
	db.semester = logical
	db.databaseName = dbName
	return logical
}

// SetSemesterReadOnly sets the read-only flag of the current database (upsert,
// preserving the schema version).
func (db *DB) SetSemesterReadOnly(ctx context.Context, readOnly bool) error {
	collection := db.Client.Database(db.databaseName).Collection(collectionSemesterMeta)
	_, err := collection.UpdateOne(ctx, bson.M{},
		bson.M{"$set": bson.M{"readOnly": readOnly}},
		options.Update().SetUpsert(true))
	if err != nil {
		log.Error().Err(err).Bool("readOnly", readOnly).Msg("cannot set read-only flag")
		return err
	}
	return nil
}
