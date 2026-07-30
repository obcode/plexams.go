package db_test

import (
	"context"
	"testing"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/internal/mongotest"
	"go.mongodb.org/mongo-driver/bson"
)

// TestReplaceAll covers the wholesale collection swap that replaced the untyped
// collection name travelling through context.Value.
func TestReplaceAll(t *testing.T) {
	d := mongotest.NewDB(t)
	ctx := context.Background()
	coll := d.Client.Database(d.DatabaseName()).Collection("zpastudents")

	if err := d.ReplaceAll(ctx, db.TargetZPAStudents, []interface{}{
		bson.M{"mtknr": "a"}, bson.M{"mtknr": "b"}, bson.M{"mtknr": "c"},
	}); err != nil {
		t.Fatal(err)
	}
	if n, _ := coll.CountDocuments(ctx, bson.M{}); n != 3 {
		t.Fatalf("got %d documents, want 3", n)
	}

	// A second call replaces rather than appends.
	if err := d.ReplaceAll(ctx, db.TargetZPAStudents, []interface{}{bson.M{"mtknr": "d"}}); err != nil {
		t.Fatal(err)
	}
	n, err := coll.CountDocuments(ctx, bson.M{})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("got %d documents, want 1 — the previous contents should be gone", n)
	}
	if n, _ := coll.CountDocuments(ctx, bson.M{"mtknr": "d"}); n != 1 {
		t.Error("the new document is missing")
	}

	// An empty slice clears the collection instead of failing on an empty InsertMany.
	if err := d.ReplaceAll(ctx, db.TargetZPAStudents, nil); err != nil {
		t.Fatalf("clearing must not fail: %v", err)
	}
	if n, _ := coll.CountDocuments(ctx, bson.M{}); n != 0 {
		t.Errorf("got %d documents, want 0", n)
	}
}

// TestReplaceAllTargetsDistinctCollections guards the constants against pointing at the
// same collection, which the untyped string version could not catch.
func TestReplaceAllTargetsDistinctCollections(t *testing.T) {
	d := mongotest.NewDB(t)
	ctx := context.Background()

	targets := []db.ReplaceTarget{
		db.TargetZPAStudents, db.TargetInvigilatorRequirements,
		db.TargetSelfInvigilations, db.TargetOtherInvigilations,
	}
	for i, target := range targets {
		if err := d.ReplaceAll(ctx, target, []interface{}{bson.M{"i": i}}); err != nil {
			t.Fatalf("%s: %v", target, err)
		}
	}
	for i, target := range targets {
		n, err := d.Client.Database(d.DatabaseName()).Collection(string(target)).
			CountDocuments(ctx, bson.M{"i": i})
		if err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("%s does not hold its own document — two targets share a collection", target)
		}
	}
}
