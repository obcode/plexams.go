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
	var idx []bson.M
	if err := cur.All(ctx, &idx); err != nil {
		t.Fatal(err)
	}
	if len(idx) != 2 { // _id_ plus our ancode index
		t.Errorf("got %d indexes on plan, want 2 (_id_ and ancode)", len(idx))
	}
}
