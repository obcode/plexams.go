package db

import (
	"context"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// SemesterMeta is per-database metadata (not planning config): the data schema
// version (compatibility) and the read-only flag.
type SemesterMeta struct {
	SchemaVersion int  `bson:"schemaVersion"`
	ReadOnly      bool `bson:"readOnly"`
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

// EnsureSchemaVersion writes the schema version into the current database's meta if
// no meta exists yet (stamping a freshly set-up semester). Existing meta is left
// untouched (it keeps its version and readOnly flag).
func (db *DB) EnsureSchemaVersion(ctx context.Context, version int) error {
	meta, err := db.GetSemesterMeta(ctx)
	if err != nil {
		return err
	}
	if meta != nil {
		return nil
	}
	collection := db.Client.Database(db.databaseName).Collection(collectionSemesterMeta)
	if _, err := collection.InsertOne(ctx, &SemesterMeta{SchemaVersion: version, ReadOnly: false}); err != nil {
		log.Error().Err(err).Msg("cannot stamp schema version")
		return err
	}
	return nil
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
