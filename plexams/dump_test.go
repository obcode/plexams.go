package plexams

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// TestExtJSONRoundTripPreservesTypes ensures the canonical extended-JSON encoding
// used by the dumps round-trips the BSON types we rely on (ObjectID, date, ints,
// nested docs/arrays), so a restore re-inserts the exact same values.
func TestExtJSONRoundTripPreservesTypes(t *testing.T) {
	oid := primitive.NewObjectID()
	now := primitive.NewDateTimeFromTime(time.Date(2026, 7, 6, 10, 30, 0, 0, time.UTC))
	orig := []bson.M{
		{
			"_id":    oid,
			"ancode": int32(1234),
			"count":  int64(9_000_000_000),
			"module": "Analysis",
			"date":   now,
			"rooms":  bson.A{"R1.234", "T3.015"},
			"nested": bson.M{"day": int32(2), "slot": int32(3)},
		},
	}

	data, err := bson.MarshalExtJSON(collectionDump{Documents: orig}, true, false)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var envelope collectionDump
	if err := bson.UnmarshalExtJSON(data, true, &envelope); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := envelope.Documents
	if len(got) != 1 {
		t.Fatalf("expected 1 doc, got %d", len(got))
	}
	d := got[0]

	if v, ok := d["_id"].(primitive.ObjectID); !ok || v != oid {
		t.Errorf("_id not preserved as ObjectID: %T %v", d["_id"], d["_id"])
	}
	if v, ok := d["ancode"].(int32); !ok || v != 1234 {
		t.Errorf("ancode not preserved as int32: %T %v", d["ancode"], d["ancode"])
	}
	if v, ok := d["count"].(int64); !ok || v != 9_000_000_000 {
		t.Errorf("count not preserved as int64: %T %v", d["count"], d["count"])
	}
	if v, ok := d["date"].(primitive.DateTime); !ok || v != now {
		t.Errorf("date not preserved as DateTime: %T %v", d["date"], d["date"])
	}
	if nested, ok := d["nested"].(bson.M); !ok {
		t.Errorf("nested not preserved as bson.M: %T", d["nested"])
	} else if v, ok := nested["slot"].(int32); !ok || v != 3 {
		t.Errorf("nested.slot not preserved: %T %v", nested["slot"], nested["slot"])
	}
}

// TestDatasetDumpRoundTrip ensures the multi-collection dataset envelope round-trips.
func TestDatasetDumpRoundTrip(t *testing.T) {
	orig := datasetDump{
		Manifest: datasetManifest{Dataset: "external-exams", Semester: "2026 SS", Format: 1, Counts: map[string]int{"non_zpaexams": 1, "plan": 1}},
		Collections: map[string][]bson.M{
			"non_zpaexams": {{"ancode": int32(9001), "module": "Extern"}},
			"plan":         {{"ancode": int32(9001), "daynumber": int32(0), "slotnumber": int32(0)}},
		},
	}
	data, err := bson.MarshalExtJSON(&orig, true, false)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got datasetDump
	if err := bson.UnmarshalExtJSON(data, true, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Manifest.Dataset != "external-exams" {
		t.Errorf("dataset name lost: %q", got.Manifest.Dataset)
	}
	if len(got.Collections["plan"]) != 1 {
		t.Fatalf("plan collection lost")
	}
	if a, ok := toInt(got.Collections["plan"][0]["ancode"]); !ok || a != 9001 {
		t.Errorf("plan ancode lost: %v", got.Collections["plan"][0]["ancode"])
	}
}

func TestAncodeHelpers(t *testing.T) {
	docs := []bson.M{
		{"ancode": int32(1)},
		{"ancode": int64(2)},
		{"ancode": 3.0},
		{"module": "no ancode"},
	}
	set := ancodeSet(docs)
	for _, a := range []int{1, 2, 3} {
		if !set[a] {
			t.Errorf("ancode %d missing from set", a)
		}
	}
	if len(set) != 3 {
		t.Errorf("expected 3 ancodes, got %d", len(set))
	}

	plan := []bson.M{{"ancode": int32(1)}, {"ancode": int32(99)}, {"ancode": int32(2)}}
	filtered := filterByAncode(plan, set)
	if len(filtered) != 2 {
		t.Errorf("expected 2 filtered docs, got %d", len(filtered))
	}

	union := unionAncodes(map[int]bool{1: true, 2: true}, map[int]bool{2: true, 5: true})
	if len(union) != 3 {
		t.Errorf("expected union of 3, got %v", union)
	}
}
