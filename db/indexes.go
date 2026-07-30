package db

import (
	"context"
	"errors"
	"fmt"
	"strings"

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

// uniqueIndex builds a unique index over keys.
func uniqueIndex(keys bson.D) mongo.IndexModel {
	return mongo.IndexModel{Keys: keys, Options: options.Index().SetUnique(true)}
}

// lookupIndex builds a plain non-unique index used to speed up a read path.
func lookupIndex(keys bson.D) mongo.IndexModel {
	return mongo.IndexModel{Keys: keys}
}

// semesterIndexes are the indexes of the current semester database.
//
// Deliberately NOT unique: studentregs_<PROG> on {AnCode, MTKNR}. The Primuss source
// data really does contain a student registered twice for the same exam — present in
// the current semester and in the freshly imported workspace, so it is not a legacy
// artefact. A unique index there would reject the import rather than protect it; the
// duplicate belongs in the validation report instead.
func (db *DB) semesterIndexes() []indexSpec {
	return []indexSpec{
		// One plan entry per exam — the invariant ValidateDB checks by hand.
		{collectionNamePlan, uniqueIndex(bson.D{{Key: "ancode", Value: 1}})},
		// Matches the upsert filter in UpsertJointLink.
		{collectionJointLinks, uniqueIndex(bson.D{
			{Key: "program", Value: 1},
			{Key: "primussancode", Value: 1},
		})},
		// Matches the upsert filter in AddAncode.
		{collectionPrimussAncodes, uniqueIndex(bson.D{
			{Key: "ancode", Value: 1},
			{Key: "primussancode.program", Value: 1},
		})},
		// Read paths: rooms of one exam, and all rooms at one time.
		{collectionRoomsPlanned, lookupIndex(bson.D{{Key: "ancode", Value: 1}})},
		{collectionRoomsPlanned, lookupIndex(bson.D{{Key: "starttime", Value: 1}})},
		// MutationLog sorts by time descending.
		{collectionMutationLog, lookupIndex(bson.D{{Key: "time", Value: -1}})},
	}
}

// EnsureIndexes creates the indexes of the current semester database and of the global
// one. Creating an index that already exists with the same definition is a no-op; one
// that exists under the same name with a DIFFERENT definition (this code's own, from an
// earlier version) is replaced.
//
// It is best-effort by design: an index whose collection already holds violating data
// cannot be created, and that must not keep the server from starting. Such a failure is
// logged as a warning naming the collection and the index, so the conflict can be
// resolved (ValidateDB reports the same violations) and the index picked up on the next
// start or semester switch.
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
	db.ensureIndex(ctx, db.globalDatabase().Collection(collectionNameNTAs),
		uniqueIndex(bson.D{{Key: "mtknr", Value: 1}}))
}

// MongoDB error codes for "an index of this name already exists, but with a different
// definition". Both mean the stored index has to be replaced, not that the data is bad.
const (
	errIndexOptionsConflict  = 85
	errIndexKeySpecsConflict = 86
)

func (db *DB) ensureIndex(ctx context.Context, collection *mongo.Collection, model mongo.IndexModel) {
	_, err := collection.Indexes().CreateOne(ctx, model)
	if err == nil {
		return
	}

	logger := log.With().
		Str("database", collection.Database().Name()).
		Str("collection", collection.Name()).
		Str("index", indexNameOf(model)).Logger()

	if !isIndexSpecConflict(err) {
		logger.Warn().Err(err).
			Msg("cannot create index — existing data violates it, fix the duplicates and restart")
		return
	}

	// Same name, different definition: an index this code created in an earlier version.
	// It would conflict on every single start, so replace it rather than warn forever.
	// Between the drop and the create the collection is briefly unconstrained, which is
	// acceptable at startup.
	name := indexNameOf(model)
	if _, dropErr := collection.Indexes().DropOne(ctx, name); dropErr != nil {
		logger.Warn().Err(dropErr).Msg("cannot drop the outdated index, leaving it in place")
		return
	}
	if _, createErr := collection.Indexes().CreateOne(ctx, model); createErr != nil {
		logger.Warn().Err(createErr).
			Msg("dropped the outdated index but cannot create the new one — the collection now has no such index")
		return
	}
	logger.Info().Msg("replaced an outdated index definition")
}

func isIndexSpecConflict(err error) bool {
	var serverErr mongo.ServerError
	if !errors.As(err, &serverErr) {
		return false
	}
	return serverErr.HasErrorCode(errIndexOptionsConflict) ||
		serverErr.HasErrorCode(errIndexKeySpecsConflict)
}

// indexNameOf reproduces the name MongoDB derives from the keys ("ancode_1",
// "program_1_primussancode_1", "time_-1"), so an existing index can be dropped by name.
// The models here never set an explicit name, so this is always the stored one.
func indexNameOf(model mongo.IndexModel) string {
	keys, ok := model.Keys.(bson.D)
	if !ok {
		return ""
	}
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s_%v", k.Key, k.Value))
	}
	return strings.Join(parts, "_")
}
