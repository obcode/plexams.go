package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/mongotest"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func planCollection(d *db.DB) *mongo.Collection {
	return d.Client.Database(d.DatabaseName()).Collection("plan")
}

// TestEnsureIndexesRejectsDuplicatePlanEntries verifies the invariant ValidateDB
// checks by hand is now enforced by the database: one plan entry per exam.
func TestEnsureIndexesRejectsDuplicatePlanEntries(t *testing.T) {
	d := mongotest.NewDB(t)
	ctx := context.Background()
	d.EnsureIndexes(ctx)

	st := time.Date(2026, 7, 6, 8, 30, 0, 0, time.Local)
	if _, err := planCollection(d).InsertOne(ctx, model.PlanEntry{Ancode: 7, Starttime: &st}); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	_, err := planCollection(d).InsertOne(ctx, model.PlanEntry{Ancode: 7, Starttime: &st})
	if err == nil {
		t.Fatal("second plan entry with the same ancode was accepted, want duplicate key error")
	}
	if !mongo.IsDuplicateKeyError(err) {
		t.Errorf("got %v, want a duplicate key error", err)
	}
}

// TestEnsureIndexesToleratesViolatingData verifies the best-effort contract: a
// collection that already holds duplicates must not keep the server from starting.
func TestEnsureIndexesToleratesViolatingData(t *testing.T) {
	d := mongotest.NewDB(t)
	ctx := context.Background()

	st := time.Date(2026, 7, 6, 8, 30, 0, 0, time.Local)
	dupes := []any{
		model.PlanEntry{Ancode: 7, Starttime: &st},
		model.PlanEntry{Ancode: 7, Starttime: &st},
	}
	if _, err := planCollection(d).InsertMany(ctx, dupes); err != nil {
		t.Fatal(err)
	}

	d.EnsureIndexes(ctx) // must not panic and must return

	// The duplicates are still there — EnsureIndexes never deletes data.
	n, err := planCollection(d).CountDocuments(ctx, bson.M{"ancode": 7})
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("got %d plan entries, want the 2 pre-existing ones untouched", n)
	}
}

// TestEnsureIndexesIsIdempotent verifies it can run on every start and semester switch.
func TestEnsureIndexesIsIdempotent(t *testing.T) {
	d := mongotest.NewDB(t)
	ctx := context.Background()

	d.EnsureIndexes(ctx)
	d.EnsureIndexes(ctx)

	cur, err := planCollection(d).Indexes().List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var idx []map[string]any
	if err := cur.All(ctx, &idx); err != nil {
		t.Fatal(err)
	}
	if len(idx) != 2 { // _id_ plus our ancode index
		t.Errorf("got %d indexes on plan, want 2 (_id_ and ancode)", len(idx))
	}
}

// TestIndexesSurviveResetAndReimport guards the paths that clear a collection instead of
// dropping it: a drop would also discard the indexes EnsureIndexes created, which would
// only come back on the next start.
func TestIndexesSurviveResetAndReimport(t *testing.T) {
	d := mongotest.NewDB(t)
	ctx := context.Background()

	if _, err := d.ReplaceRawCollection(ctx, "studentregs_WT", []map[string]any{
		{"AnCode": 100, "MTKNR": "a"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ReplaceRawCollection(ctx, "exams_WT", []map[string]any{{"AnCode": 100}}); err != nil {
		t.Fatal(err)
	}
	d.EnsureIndexes(ctx)

	regsBefore := len(indexNames(t, d, "studentregs_WT"))
	roomsBefore := len(indexNames(t, d, "rooms_planned"))
	if regsBefore < 2 || roomsBefore < 2 {
		t.Fatalf("precondition: expected indexes, got %d on studentregs_WT and %d on rooms_planned",
			regsBefore, roomsBefore)
	}

	if err := d.ResetPlannedRooms(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ReplaceRawCollection(ctx, "studentregs_WT", []map[string]any{
		{"AnCode": 101, "MTKNR": "b"},
	}); err != nil {
		t.Fatal(err)
	}

	if got := len(indexNames(t, d, "rooms_planned")); got != roomsBefore {
		t.Errorf("rooms_planned has %d indexes after the reset, want %d", got, roomsBefore)
	}
	if got := len(indexNames(t, d, "studentregs_WT")); got != regsBefore {
		t.Errorf("studentregs_WT has %d indexes after the re-import, want %d", got, regsBefore)
	}
}

// TestEnsureIndexesReplacesOutdatedDefinition reproduces what happened in the field: an
// earlier version created plan(ancode) as a PARTIAL unique index. The simplified plain
// unique index carries the same auto-generated name, so MongoDB rejected it with
// IndexKeySpecsConflict on every single start.
func TestEnsureIndexesReplacesOutdatedDefinition(t *testing.T) {
	d := mongotest.NewDB(t)
	ctx := context.Background()

	// The index definition of the earlier version.
	if _, err := planCollection(d).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "ancode", Value: 1}},
		Options: options.Index().SetUnique(true).
			SetPartialFilterExpression(bson.M{"ancode": bson.M{"$exists": true}}),
	}); err != nil {
		t.Fatal(err)
	}

	d.EnsureIndexes(ctx)

	cur, err := planCollection(d).Indexes().List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var specs []bson.M
	if err := cur.All(ctx, &specs); err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, s := range specs {
		if s["name"] != "ancode_1" {
			continue
		}
		found = true
		if _, stillPartial := s["partialFilterExpression"]; stillPartial {
			t.Error("ancode_1 is still the outdated partial index")
		}
		if s["unique"] != true {
			t.Errorf("ancode_1 is not unique: %v", s)
		}
	}
	if !found {
		t.Fatalf("ancode_1 is gone entirely: %v", specs)
	}

	// And it constrains, so the replacement really took effect.
	st := time.Date(2026, 7, 6, 8, 30, 0, 0, time.Local)
	if _, err := planCollection(d).InsertOne(ctx, model.PlanEntry{Ancode: 7, Starttime: &st}); err != nil {
		t.Fatal(err)
	}
	if _, err := planCollection(d).InsertOne(ctx, model.PlanEntry{Ancode: 7, Starttime: &st}); err == nil {
		t.Error("duplicate ancode accepted after the index replacement")
	}
}
