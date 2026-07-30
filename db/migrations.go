package db

import (
	"context"
	"fmt"
	"regexp"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	// CurrentSchemaVersion is the layout version this code writes. Bump it together
	// with a new entry in migrations().
	CurrentSchemaVersion = 2
	// MinSupportedSchemaVersion is the oldest layout this code can still work with.
	MinSupportedSchemaVersion = 1
)

// migration is one step that brings a semester database up to version. Steps must be
// idempotent: a crash between running a step and stamping its version re-runs it.
type migration struct {
	version int
	name    string
	run     func(*DB, context.Context) error
}

// migrations are applied in ascending version order to every database whose stored
// version is lower. Never edit or reorder a released entry — add a new one.
func migrations() []migration {
	return []migration{
		{
			version: 2,
			name:    "rename legacy collections to their current names",
			run:     (*DB).migrateLegacyCollectionNames,
		},
	}
}

// Migrate brings the current database up to CurrentSchemaVersion, stamping the
// version after each step so an interrupted run resumes where it stopped.
//
// Databases without meta are left alone: those predate schema versioning (and the
// slotless refactor) and are archives, not planning targets. Read-only databases are
// left alone too — migrating them would defeat the protection; they stay readable
// because MinSupportedSchemaVersion still covers them.
func (db *DB) Migrate(ctx context.Context) error {
	meta, err := db.GetSemesterMeta(ctx)
	if err != nil {
		return err
	}
	if meta == nil {
		return nil
	}
	if meta.SchemaVersion >= CurrentSchemaVersion {
		if meta.SchemaVersion > CurrentSchemaVersion {
			log.Warn().Int("database", meta.SchemaVersion).Int("code", CurrentSchemaVersion).
				Str("database name", db.databaseName).
				Msg("database was written by a newer version of plexams, not migrating")
		}
		return nil
	}
	if meta.ReadOnly {
		log.Warn().Int("from", meta.SchemaVersion).Int("to", CurrentSchemaVersion).
			Str("database", db.databaseName).
			Msg("database is read-only, skipping migrations — unprotect it to migrate")
		return nil
	}

	for _, m := range migrations() {
		if m.version <= meta.SchemaVersion {
			continue
		}
		log.Info().Int("version", m.version).Str("migration", m.name).
			Str("database", db.databaseName).Msg("migrating")
		if err := m.run(db, ctx); err != nil {
			return fmt.Errorf("migration %d (%s) failed: %w", m.version, m.name, err)
		}
		if err := db.setSchemaVersion(ctx, m.version); err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) setSchemaVersion(ctx context.Context, version int) error {
	collection := db.Client.Database(db.databaseName).Collection(collectionSemesterMeta)
	_, err := collection.UpdateOne(ctx, bson.M{},
		bson.M{"$set": bson.M{"schemaVersion": version}})
	if err != nil {
		log.Error().Err(err).Int("version", version).Msg("cannot stamp schema version")
	}
	return err
}

// legacyMucdaiProgram matches the per-program joint collections under their old name.
// The program code is 2-4 uppercase letters, so "mucdai_links" cannot match.
var legacyMucdaiProgram = regexp.MustCompile(`^mucdai_[A-Z]{2,4}(-[BM])?$`)

// migrateLegacyCollectionNames renames collections the code stopped reading when it
// was renamed, leaving their data unreachable: the explicit joint links and per-program
// joint exams (mucdai_* → joint_*, see the MUC.DAI generalization) and the assembled
// exams cache and its dirty flag (generated_exams* → assembled_exams*).
//
// All of these are pure renames — the stored documents already carry the current field
// layout, verified against the existing semester databases — so no document is touched.
func (db *DB) migrateLegacyCollectionNames(ctx context.Context) error {
	renames := map[string]string{
		"mucdai_links":          collectionJointLinks,
		"generated_exams":       collectionAssembledExams,
		"generated_exams_state": collectionAssembledExamsState,
	}

	names, err := db.Client.Database(db.databaseName).ListCollectionNames(ctx, bson.M{})
	if err != nil {
		return err
	}
	for _, name := range names {
		if legacyMucdaiProgram.MatchString(name) {
			renames[name] = "joint_" + name[len("mucdai_"):]
		}
	}

	for src, dst := range renames {
		if err := db.renameCollection(ctx, src, dst); err != nil {
			return err
		}
	}
	return nil
}

// renameCollection renames src to dst within the current database.
//
// An absent src is a no-op. A dst that already holds documents is left untouched and
// logged, so a rename can never overwrite real data; an empty dst is dropped first,
// which is exactly the state an earlier partial rename leaves behind (the code creates
// the new collection on first write while the data sits under the old name).
func (db *DB) renameCollection(ctx context.Context, src, dst string) error {
	database := db.Client.Database(db.databaseName)

	present, err := database.ListCollectionNames(ctx, bson.M{"name": bson.M{"$in": []string{src, dst}}})
	if err != nil {
		return err
	}
	var srcExists, dstExists bool
	for _, n := range present {
		switch n {
		case src:
			srcExists = true
		case dst:
			dstExists = true
		}
	}
	if !srcExists {
		return nil
	}

	if dstExists {
		n, err := database.Collection(dst).CountDocuments(ctx, bson.M{})
		if err != nil {
			return err
		}
		if n > 0 {
			log.Warn().Str("from", src).Str("to", dst).Int64("documents in target", n).
				Str("database", db.databaseName).
				Msg("not renaming, target already holds data — resolve by hand")
			return nil
		}
		if err := database.Collection(dst).Drop(ctx); err != nil {
			return err
		}
	}

	qualified := func(c string) string { return db.databaseName + "." + c }
	err = db.Client.Database("admin").RunCommand(ctx, bson.D{
		{Key: "renameCollection", Value: qualified(src)},
		{Key: "to", Value: qualified(dst)},
	}).Err()
	if err != nil {
		log.Error().Err(err).Str("from", src).Str("to", dst).Msg("cannot rename collection")
		return err
	}
	log.Info().Str("from", src).Str("to", dst).Str("database", db.databaseName).
		Msg("renamed legacy collection")
	return nil
}
