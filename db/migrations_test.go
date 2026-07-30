package db_test

import (
	"context"
	"testing"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/internal/mongotest"
	"go.mongodb.org/mongo-driver/bson"
)

// seedV1 makes the test database look like an existing one: stamped schema version 1,
// with the given legacy collections filled.
func seedV1(t *testing.T, d *db.DB, collections map[string]int) {
	t.Helper()
	ctx := context.Background()
	if err := d.EnsureMeta(ctx, 1); err != nil {
		t.Fatal(err)
	}
	for name, n := range collections {
		coll := d.Client.Database(d.DatabaseName()).Collection(name)
		docs := make([]any, 0, n)
		for i := range n {
			docs = append(docs, bson.M{"i": i})
		}
		if len(docs) == 0 {
			if err := d.Client.Database(d.DatabaseName()).CreateCollection(ctx, name); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if _, err := coll.InsertMany(ctx, docs); err != nil {
			t.Fatal(err)
		}
	}
}

func countIn(t *testing.T, d *db.DB, collection string) int64 {
	t.Helper()
	n, err := d.Client.Database(d.DatabaseName()).Collection(collection).
		CountDocuments(context.Background(), bson.M{})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func exists(t *testing.T, d *db.DB, collection string) bool {
	t.Helper()
	names, err := d.Client.Database(d.DatabaseName()).
		ListCollectionNames(context.Background(), bson.M{"name": collection})
	if err != nil {
		t.Fatal(err)
	}
	return len(names) == 1
}

// TestMigrateRenamesLegacyCollections covers the plain case: the code stopped reading
// these names, so their data was unreachable until the rename.
func TestMigrateRenamesLegacyCollections(t *testing.T) {
	d := mongotest.NewDB(t)
	ctx := context.Background()
	seedV1(t, d, map[string]int{
		"mucdai_links":          42,
		"mucdai_DE":             19,
		"generated_exams":       147,
		"generated_exams_state": 1,
	})

	if err := d.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	for coll, want := range map[string]int64{
		"joint_links":           42,
		"joint_DE":              19,
		"assembled_exams":       147,
		"assembled_exams_state": 1,
	} {
		if got := countIn(t, d, coll); got != want {
			t.Errorf("%s has %d documents, want %d", coll, got, want)
		}
	}
	for _, coll := range []string{"mucdai_links", "mucdai_DE", "generated_exams", "generated_exams_state"} {
		if exists(t, d, coll) {
			t.Errorf("%s still exists after the migration", coll)
		}
	}

	meta, err := d.GetSemesterMeta(ctx)
	if err != nil || meta == nil {
		t.Fatalf("GetSemesterMeta: %v", err)
	}
	if meta.SchemaVersion != db.CurrentSchemaVersion {
		t.Errorf("schema version is %d, want %d", meta.SchemaVersion, db.CurrentSchemaVersion)
	}
}

// TestMigrateDropsEmptyTargetBeforeRenaming is the state found in the real databases:
// the code created the new collection on first write while the data stayed under the
// old name, leaving an empty target that must not block the rename.
func TestMigrateDropsEmptyTargetBeforeRenaming(t *testing.T) {
	d := mongotest.NewDB(t)
	ctx := context.Background()
	seedV1(t, d, map[string]int{"mucdai_links": 42, "joint_links": 0})

	if !exists(t, d, "joint_links") {
		t.Fatal("precondition: empty joint_links was not created")
	}

	if err := d.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	if got := countIn(t, d, "joint_links"); got != 42 {
		t.Errorf("joint_links has %d documents, want the 42 recovered ones", got)
	}
	if exists(t, d, "mucdai_links") {
		t.Error("mucdai_links still exists after the migration")
	}
}

// TestMigrateKeepsNonEmptyTarget verifies a rename can never overwrite real data.
func TestMigrateKeepsNonEmptyTarget(t *testing.T) {
	d := mongotest.NewDB(t)
	ctx := context.Background()
	seedV1(t, d, map[string]int{"mucdai_links": 42, "joint_links": 7})

	if err := d.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	if got := countIn(t, d, "joint_links"); got != 7 {
		t.Errorf("joint_links has %d documents, want the 7 existing ones untouched", got)
	}
	if got := countIn(t, d, "mucdai_links"); got != 42 {
		t.Errorf("mucdai_links has %d documents, want it left in place for manual resolution", got)
	}
}

// TestMigrateSkipsReadOnly verifies the protection wins over the migration.
func TestMigrateSkipsReadOnly(t *testing.T) {
	d := mongotest.NewDB(t)
	ctx := context.Background()
	seedV1(t, d, map[string]int{"mucdai_links": 42})
	if err := d.SetSemesterReadOnly(ctx, true); err != nil {
		t.Fatal(err)
	}

	if err := d.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	if got := countIn(t, d, "mucdai_links"); got != 42 {
		t.Errorf("mucdai_links has %d documents, want 42 — a read-only database must not be migrated", got)
	}
	meta, err := d.GetSemesterMeta(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if meta.SchemaVersion != 1 {
		t.Errorf("schema version is %d, want it left at 1", meta.SchemaVersion)
	}
}

// TestMigrateIsIdempotent verifies it can run on every start and semester switch.
func TestMigrateIsIdempotent(t *testing.T) {
	d := mongotest.NewDB(t)
	ctx := context.Background()
	seedV1(t, d, map[string]int{"mucdai_links": 42})

	for i := range 3 {
		if err := d.Migrate(ctx); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	if got := countIn(t, d, "joint_links"); got != 42 {
		t.Errorf("joint_links has %d documents, want 42", got)
	}
}

// TestMigrateWithoutMetaDoesNothing covers the pre-versioning archives (2021-WS …
// 2023-WS), which have no semester_meta and are not planning targets.
func TestMigrateWithoutMetaDoesNothing(t *testing.T) {
	d := mongotest.NewDB(t)
	ctx := context.Background()
	coll := d.Client.Database(d.DatabaseName()).Collection("mucdai_links")
	if _, err := coll.InsertOne(ctx, bson.M{"i": 1}); err != nil {
		t.Fatal(err)
	}

	if err := d.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	if !exists(t, d, "mucdai_links") {
		t.Error("migrated a database that has no semester_meta")
	}
	if meta, err := d.GetSemesterMeta(ctx); err != nil || meta != nil {
		t.Errorf("Migrate created meta out of nothing: %v, %v", meta, err)
	}
}

// TestMigrateSkipsNewerDatabase guards against a downgrade silently rewriting data
// written by a newer build.
func TestMigrateSkipsNewerDatabase(t *testing.T) {
	d := mongotest.NewDB(t)
	ctx := context.Background()
	seedV1(t, d, map[string]int{"mucdai_links": 42})
	if err := d.EnsureMeta(ctx, 1); err != nil {
		t.Fatal(err)
	}
	meta := d.Client.Database(d.DatabaseName()).Collection("semester_meta")
	if _, err := meta.UpdateOne(ctx, bson.M{},
		bson.M{"$set": bson.M{"schemaVersion": db.CurrentSchemaVersion + 1}}); err != nil {
		t.Fatal(err)
	}

	if err := d.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	if got := countIn(t, d, "mucdai_links"); got != 42 {
		t.Errorf("mucdai_links has %d documents, want it untouched", got)
	}
}

// TestMigrateLeavesMucdaiLinksOutOfTheProgramPattern guards the regex: "links" is not
// a program code, so mucdai_links must be renamed to joint_links and never joint_links
// via the per-program rule.
func TestMigrateLeavesMucdaiLinksOutOfTheProgramPattern(t *testing.T) {
	d := mongotest.NewDB(t)
	ctx := context.Background()
	seedV1(t, d, map[string]int{"mucdai_links": 1, "mucdai_GS": 2, "mucdai_DC-B": 3})

	if err := d.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	for coll, want := range map[string]int64{"joint_links": 1, "joint_GS": 2, "joint_DC-B": 3} {
		if got := countIn(t, d, coll); got != want {
			t.Errorf("%s has %d documents, want %d", coll, got, want)
		}
	}
}
