package db_test

import (
	"context"
	"testing"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/mongotest"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func room(name string, ancode int) *model.PlannedRoom {
	return &model.PlannedRoom{RoomName: name, Ancode: ancode, Duration: 90}
}

// TestReplacePlannedRoomsRollsBackOnFailure pins the point of the transaction: a
// failing insert must not leave the room plan wiped. Both outcomes are asserted, so the
// documented fallback on a standalone mongod stays honest rather than untested.
func TestReplacePlannedRoomsRollsBackOnFailure(t *testing.T) {
	d := mongotest.NewDB(t)
	ctx := context.Background()

	if err := d.ReplacePlannedRooms(ctx, []*model.PlannedRoom{
		room("R1.001", 1), room("R1.002", 2), room("R1.003", 3),
	}); err != nil {
		t.Fatal(err)
	}

	// Force the second write of the next call to fail: a unique index on roomname makes
	// InsertMany reject a batch containing the same room twice.
	coll := d.Client.Database(d.DatabaseName()).Collection("rooms_planned")
	if _, err := coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "roomname", Value: 1}},
		Options: options.Index().SetUnique(true),
	}); err != nil {
		t.Fatal(err)
	}

	err := d.ReplacePlannedRooms(ctx, []*model.PlannedRoom{room("X1.001", 9), room("X1.001", 10)})
	if err == nil {
		t.Fatal("ReplacePlannedRooms accepted a duplicate room name, cannot test the rollback")
	}

	survivors, err := coll.CountDocuments(ctx,
		bson.M{"roomname": bson.M{"$in": []string{"R1.001", "R1.002", "R1.003"}}})
	if err != nil {
		t.Fatal(err)
	}
	if d.SupportsTransactions() {
		if survivors != 3 {
			t.Errorf("got %d of the original rooms, want all 3 restored by the rollback", survivors)
		}
		return
	}
	// Standalone mongod: the delete committed and the insert got half-way through
	// (InsertMany is ordered), leaving the room plan in a partial state. That is exactly
	// the damage the transaction prevents — documented here rather than silently
	// tolerated.
	if survivors != 0 {
		t.Errorf("got %d of the original rooms, want 0 — without a transaction the delete stands", survivors)
	}
	total, err := coll.CountDocuments(ctx, bson.M{})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("no replica set: ran without a transaction, room plan left partial (%d documents)", total)
}

// TestReplacePlannedRoomsKeepsIndexes guards the switch from Drop to DeleteMany: a drop
// would also remove the indexes EnsureIndexes created.
func TestReplacePlannedRoomsKeepsIndexes(t *testing.T) {
	d := mongotest.NewDB(t)
	ctx := context.Background()
	d.EnsureIndexes(ctx)

	before := indexNames(t, d, "rooms_planned")
	if len(before) < 2 {
		t.Fatalf("precondition: expected EnsureIndexes to create indexes, got %v", before)
	}

	if err := d.ReplacePlannedRooms(ctx, []*model.PlannedRoom{room("R1.001", 1)}); err != nil {
		t.Fatal(err)
	}

	after := indexNames(t, d, "rooms_planned")
	if len(after) != len(before) {
		t.Errorf("indexes changed from %v to %v — did ReplacePlannedRooms drop the collection?", before, after)
	}
}

// TestReplacePlannedRoomsWithEmptyList covers the guard against InsertMany with no
// documents, which MongoDB rejects.
func TestReplacePlannedRoomsWithEmptyList(t *testing.T) {
	d := mongotest.NewDB(t)
	ctx := context.Background()

	if err := d.ReplacePlannedRooms(ctx, []*model.PlannedRoom{room("R1.001", 1)}); err != nil {
		t.Fatal(err)
	}
	if err := d.ReplacePlannedRooms(ctx, nil); err != nil {
		t.Fatalf("clearing the room plan must not fail: %v", err)
	}

	n, err := d.Client.Database(d.DatabaseName()).Collection("rooms_planned").
		CountDocuments(ctx, bson.M{})
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("got %d planned rooms, want 0", n)
	}
}

// TestAddStudentRegKeepsCounterInStep covers the drift cause: registration and counter
// are written together.
func TestAddStudentRegKeepsCounterInStep(t *testing.T) {
	d := mongotest.NewDB(t)
	ctx := context.Background()
	seedRegsAndCounts(t, d, "WT", map[int]int{100: 2}, map[int]int{100: 2})

	if _, err := d.RemoveStudentReg(ctx, "WT", 100, "student-a"); err != nil {
		t.Fatal(err)
	}

	got, err := d.StudentRegsCountMismatches(ctx, "WT")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("got %+v, want the counter to have followed the removal", got)
	}
}

func indexNames(t *testing.T, d *db.DB, collection string) []string {
	t.Helper()
	cur, err := d.Client.Database(d.DatabaseName()).Collection(collection).
		Indexes().List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var specs []struct{ Name string }
	if err := cur.All(context.Background(), &specs); err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(specs))
	for _, s := range specs {
		names = append(names, s.Name)
	}
	return names
}
