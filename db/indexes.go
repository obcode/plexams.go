package db

import (
	"context"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// indexSpec is one index we want to exist, with the collection it belongs to.
type indexSpec struct {
	collection string
	model      mongo.IndexModel
}

// uniqueIndex builds a unique index over keys. onlyIfPresent limits it to documents
// that actually have the first key — needed because pre-slotless semester databases
// still hold plan entries without an "ancode" field, which would all collide on null.
func uniqueIndex(keys bson.D, onlyIfPresent string) mongo.IndexModel {
	opts := options.Index().SetUnique(true)
	if onlyIfPresent != "" {
		opts.SetPartialFilterExpression(bson.M{onlyIfPresent: bson.M{"$exists": true}})
	}
	return mongo.IndexModel{Keys: keys, Options: opts}
}

// lookupIndex builds a plain non-unique index used to speed up a read path.
func lookupIndex(keys bson.D) mongo.IndexModel {
	return mongo.IndexModel{Keys: keys}
}

// semesterIndexes are the indexes of the current semester database.
//
// Deliberately NOT unique: studentregs_<PROG> on {AnCode, MTKNR}. The Primuss source
// data really does contain a student registered twice for the same exam (seen in
// 2025-WS, carried into 2026-SS), so a unique index there would reject the import
// rather than protect it. The duplicate belongs in the validation report instead.
func (db *DB) semesterIndexes() []indexSpec {
	return []indexSpec{
		// One plan entry per exam — the invariant ValidateDB checks by hand.
		{collectionNamePlan, uniqueIndex(bson.D{{Key: "ancode", Value: 1}}, "ancode")},
		// Matches the upsert filter in UpsertJointLink.
		{collectionJointLinks, uniqueIndex(bson.D{
			{Key: "program", Value: 1},
			{Key: "primussancode", Value: 1},
		}, "")},
		// Matches the upsert filter in AddAncode.
		{collectionPrimussAncodes, uniqueIndex(bson.D{
			{Key: "ancode", Value: 1},
			{Key: "primussancode.program", Value: 1},
		}, "")},
		// Read paths: rooms of one exam, and all rooms at one time.
		{collectionRoomsPlanned, lookupIndex(bson.D{{Key: "ancode", Value: 1}})},
		{collectionRoomsPlanned, lookupIndex(bson.D{{Key: "starttime", Value: 1}})},
		// MutationLog sorts by time descending.
		{collectionMutationLog, lookupIndex(bson.D{{Key: "time", Value: -1}})},
	}
}

// EnsureIndexes creates the indexes of the current semester database and of the
// global one. It is best-effort by design: an index whose collection already holds
// violating data cannot be created, and that must not keep the server from starting.
// Such a failure is logged as a warning naming the collection, so the conflict can be
// resolved (ValidateDB reports the same violations) and the index picked up on the
// next start or semester switch. Creating an index that already exists is a no-op.
func (db *DB) EnsureIndexes(ctx context.Context) {
	for _, spec := range db.semesterIndexes() {
		db.ensureIndex(ctx, db.getCollectionSemester(spec.collection), spec.model)
	}

	// Per-study-program registrations: sped up, not constrained (see semesterIndexes).
	names, err := db.studentRegsCollectionNames(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("cannot list studentregs collections, skipping their indexes")
	}
	for _, name := range names {
		db.ensureIndex(ctx, db.getCollectionSemester(name), lookupIndex(bson.D{
			{Key: "AnCode", Value: 1},
			{Key: "MTKNR", Value: 1},
		}))
	}

	// Global database: one NTA per student.
	db.ensureIndex(ctx, db.Client.Database("plexams").Collection(collectionNameNTAs),
		uniqueIndex(bson.D{{Key: "mtknr", Value: 1}}, ""))
}

func (db *DB) ensureIndex(ctx context.Context, collection *mongo.Collection, model mongo.IndexModel) {
	if _, err := collection.Indexes().CreateOne(ctx, model); err != nil {
		log.Warn().Err(err).
			Str("database", collection.Database().Name()).
			Str("collection", collection.Name()).
			Msg("cannot create index — existing data violates it, fix the duplicates and restart")
	}
}
