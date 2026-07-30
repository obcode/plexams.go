package db_test

import (
	"context"
	"testing"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/internal/mongotest"
	"go.mongodb.org/mongo-driver/bson"
)

// seedRegsAndCounts fills studentregs_<program> with one document per (ancode, mtknr)
// and count_<program> with the given sums. Field names are the Primuss column names
// the import writes verbatim.
func seedRegsAndCounts(t *testing.T, d *db.DB, program string, regs map[int]int, counts map[int]int) {
	t.Helper()
	ctx := context.Background()
	database := d.Client.Database(d.DatabaseName())

	docs := make([]any, 0)
	for ancode, n := range regs {
		for i := range n {
			docs = append(docs, bson.M{
				"AnCode": ancode,
				"MTKNR":  "student-" + string(rune('a'+i)),
				"Stg":    program,
			})
		}
	}
	if len(docs) > 0 {
		if _, err := database.Collection("studentregs_"+program).InsertMany(ctx, docs); err != nil {
			t.Fatal(err)
		}
	}

	countDocs := make([]any, 0, len(counts))
	for ancode, sum := range counts {
		countDocs = append(countDocs, bson.M{"AnCo": ancode, "Sum": sum})
	}
	if len(countDocs) > 0 {
		if _, err := database.Collection("count_"+program).InsertMany(ctx, countDocs); err != nil {
			t.Fatal(err)
		}
	}
}

func TestStudentRegsCountMismatchesFindsNothingWhenConsistent(t *testing.T) {
	d := mongotest.NewDB(t)
	ctx := context.Background()
	seedRegsAndCounts(t, d, "WT", map[int]int{100: 3, 101: 2}, map[int]int{100: 3, 101: 2})

	got, err := d.StudentRegsCountMismatches(ctx, "WT")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("got %+v, want no mismatches", got)
	}
}

func TestStudentRegsCountMismatchesReportsDrift(t *testing.T) {
	d := mongotest.NewDB(t)
	ctx := context.Background()
	// 101 drifted (a single registration was added without the counter following),
	// 102 has no count document at all, 100 is fine.
	seedRegsAndCounts(t, d, "WT", map[int]int{100: 3, 101: 3, 102: 1}, map[int]int{100: 3, 101: 2})

	got, err := d.StudentRegsCountMismatches(ctx, "WT")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %+v, want 2 mismatches", got)
	}
	// Sorted by ancode.
	if got[0].Ancode != 101 || got[0].Stored != 3 || got[0].Recorded != 2 {
		t.Errorf("got %+v, want ancode 101 stored 3 recorded 2", got[0])
	}
	if got[1].Ancode != 102 || got[1].Stored != 1 || got[1].Recorded != db.NoCountDocument {
		t.Errorf("got %+v, want ancode 102 stored 1 and no count document", got[1])
	}
}

// TestStudentRegsPerAncodeSurvivesCountDrift is the regression this replaced: a drifted
// counter used to abort the read with an error, which took the whole exam generation
// down until the counter was repaired by hand.
func TestStudentRegsPerAncodeSurvivesCountDrift(t *testing.T) {
	d := mongotest.NewDB(t)
	ctx := context.Background()
	seedRegsAndCounts(t, d, "WT", map[int]int{100: 3}, map[int]int{100: 99})

	regs, err := d.GetPrimussStudentRegsPerAncode(ctx, "WT")
	if err != nil {
		t.Fatalf("GetPrimussStudentRegsPerAncode must not fail on a drifted counter: %v", err)
	}
	if len(regs[100]) != 3 {
		t.Errorf("got %d registrations for ancode 100, want 3", len(regs[100]))
	}
}
