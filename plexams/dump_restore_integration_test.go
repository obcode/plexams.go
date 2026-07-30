package plexams

import (
	"context"
	"testing"

	"github.com/obcode/plexams.go/internal/mongotest"
	"go.mongodb.org/mongo-driver/bson"
)

// TestRestoreDatasetRollsBackOnFailure covers the point of wrapping the restore: the
// external dataset writes three times across two collections, and a failure in the last
// one used to leave the workspace partly overwritten.
//
// The failure is provoked with the real unique index on plan(ancode): an export carrying
// the same ancode twice cannot be inserted. Both outcomes are asserted so the documented
// fallback on a standalone mongod stays honest.
func TestRestoreDatasetRollsBackOnFailure(t *testing.T) {
	d := mongotest.NewDB(t)
	ctx := context.Background()
	p := &Plexams{dbClient: d}

	// Existing state: one external exam and its plan entry.
	if _, err := d.ReplaceRawCollection(ctx, "non_zpaexams", []map[string]any{
		{"ancode": int32(9001), "module": "Extern alt"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.InsertRawDocs(ctx, "plan", []map[string]any{
		{"ancode": int32(9001), "locked": false},
	}); err != nil {
		t.Fatal(err)
	}
	d.EnsureIndexes(ctx) // brings the unique index on plan(ancode)

	// An export whose plan holds the same ancode twice: the final insert must fail.
	dump := datasetDump{
		Manifest: datasetManifest{Dataset: "external-exams", Format: 1},
		Collections: map[string][]map[string]any{
			"non_zpaexams": {{"ancode": int32(9002), "module": "Extern neu"}},
			"plan": {
				{"ancode": int32(9002), "locked": false},
				{"ancode": int32(9002), "locked": true},
			},
		},
	}
	data, err := bson.MarshalExtJSON(&dump, true, false)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := p.RestoreDataset(ctx, "external-exams", data); err == nil {
		t.Fatal("restore accepted a duplicate plan ancode, cannot test the rollback")
	}

	ext, err := d.RawCollection(ctx, "non_zpaexams")
	if err != nil {
		t.Fatal(err)
	}
	if d.SupportsTransactions() {
		if len(ext) != 1 || ext[0]["module"] != "Extern alt" {
			t.Errorf("non_zpaexams = %v, want the original document restored by the rollback", ext)
		}
		plan, err := d.RawCollection(ctx, "plan")
		if err != nil {
			t.Fatal(err)
		}
		if len(plan) != 1 {
			t.Errorf("plan holds %d entries, want the 1 original one", len(plan))
		}
		return
	}
	// Standalone mongod: the earlier writes committed, so the workspace is left
	// half-overwritten. That is the damage the transaction prevents.
	if len(ext) != 1 || ext[0]["module"] != "Extern neu" {
		t.Errorf("non_zpaexams = %v, want the replacement to have stood without a transaction", ext)
	}
	t.Log("no replica set: ran without a transaction, workspace left partly overwritten")
}
