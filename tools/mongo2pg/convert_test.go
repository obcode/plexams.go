package main

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

// The fixtures are synthetic. The real dump carries matriculation numbers, names
// and mail addresses and must not enter this repository -- the same rule as for
// TestConvertRealSammellisten.
//
// They are round-tripped through bson.Marshal/Unmarshal rather than written as Go
// maps by hand, because the bugs worth catching here live exactly in that step:
// the driver decodes arrays as primitive.A and numbers as int32, and a converter
// that only handles []any or int looks correct until it meets real BSON.
func decode(t *testing.T, m bson.M) *doc {
	t.Helper()
	raw, err := bson.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out map[string]any
	if err := bson.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return newDoc(out)
}

// TestAnnyConfigNamesSurviveDecoding is the regression test for the bug this tool
// shipped with for one dry run: BSON arrays decode to primitive.A, so asserting
// []any silently produced an empty list. The importer reported "1 imported" and
// would have written a config with no personalization names at all -- which
// decides which Anny bookings count as ours.
func TestAnnyConfigNamesSurviveDecoding(t *testing.T) {
	d := decode(t, bson.M{"personalizationnames": bson.A{"Braun", "Meier"}})
	cfg := convertAnnyConfig(d)
	if len(cfg.PersonalizationNames) != 2 ||
		cfg.PersonalizationNames[0] != "Braun" || cfg.PersonalizationNames[1] != "Meier" {
		t.Errorf("PersonalizationNames = %v, want [Braun Meier]", cfg.PersonalizationNames)
	}
}

// TestNtaGroupBecomesProgram pins the one legacy field that must NOT be dropped:
// `group` is the former name of `program`, and the two never occur together.
func TestNtaGroupBecomesProgram(t *testing.T) {
	d := decode(t, bson.M{
		"mtknr": "111", "name": "Test", "compensation": "Verlängerung 10%",
		"deltaDurationPercent": 10, "group": "IF", "from": "01.01.2020", "until": "Studium",
	})
	n, notes := convertNta(d)
	if n.Program != "IF" {
		t.Errorf("Program = %q, want IF (aus group)", n.Program)
	}
	if len(notes) == 0 {
		t.Error("the recovery must be reported, not silent")
	}
	if left := d.leftovers(); len(left) != 0 {
		t.Errorf("group must count as consumed, leftovers = %v", left)
	}
}

// A real program wins and group is then only a duplicate to drop.
func TestNtaProgramWinsOverGroup(t *testing.T) {
	d := decode(t, bson.M{
		"mtknr": "112", "name": "Test", "compensation": "x", "deltaDurationPercent": 10,
		"program": "IB", "group": "IF", "from": "a", "until": "b",
	})
	n, _ := convertNta(d)
	if n.Program != "IB" {
		t.Errorf("Program = %q, want IB", n.Program)
	}
}

// The fields the model really has lost are dropped, and the drop is reported.
func TestNtaDropsFieldsTheModelLost(t *testing.T) {
	d := decode(t, bson.M{
		"mtknr": "113", "name": "Test", "compensation": "x", "deltaDurationPercent": 10,
		"program": "IF", "from": "a", "until": "b",
		"exams": bson.A{1, 2}, "notForExams": bson.A{243},
	})
	_, notes := convertNta(d)
	if left := d.leftovers(); len(left) != 0 {
		t.Errorf("exams/notForExams must be consumed, leftovers = %v", left)
	}
	found := false
	for _, n := range notes {
		if n.Message != "" && n.Key == "113" {
			found = true
		}
	}
	if !found {
		t.Error("a discarded notForExams must be reported")
	}
}

// An NTA with neither program nor group is imported anyway, with a note. Skipping
// it would silently remove someone's accommodation.
func TestNtaWithoutProgramIsKeptAndReported(t *testing.T) {
	d := decode(t, bson.M{
		"mtknr": "114", "name": "Test", "compensation": "Verlängerung 10%", "deltaDurationPercent": 10,
	})
	n, notes := convertNta(d)
	if n.Mtknr != "114" {
		t.Fatalf("the NTA must survive, got %+v", n)
	}
	if len(notes) < 2 {
		t.Errorf("missing program AND missing validity must both be reported, got %v", notes)
	}
}

// TestRoomReadsBothSpellings covers the drift the migration keeps finding: the
// model had no bson tags, so the driver lowercased the Go field names and
// documents written before and after that carry different keys.
func TestRoomReadsBothSpellings(t *testing.T) {
	camel := decode(t, bson.M{
		"name": "R1.006", "seats": 50, "placesWithSocket": true, "hmebSeats": 20,
		"requestwith": "ANNY",
	})
	lower := decode(t, bson.M{
		"name": "R1.007", "seats": 50, "placeswithsocket": true, "hmebseats": 20,
		"requestwith": "ANNY",
	})
	for _, tc := range []struct {
		label string
		d     *doc
	}{{"camelCase", camel}, {"lowercase", lower}} {
		r, _ := convertRoom(tc.d)
		if !r.PlacesWithSocket {
			t.Errorf("%s: PlacesWithSocket not read", tc.label)
		}
		if r.HmebSeats == nil || *r.HmebSeats != 20 {
			t.Errorf("%s: HmebSeats = %v, want 20", tc.label, r.HmebSeats)
		}
		if left := tc.d.leftovers(); len(left) != 0 {
			t.Errorf("%s: unexpected leftovers %v", tc.label, left)
		}
	}
}

// needsRequest is derived in PostgreSQL, so both spellings must be consumed and
// neither may reach the model. Storing it is what let them drift apart.
func TestRoomIgnoresStoredNeedsRequest(t *testing.T) {
	d := decode(t, bson.M{
		"name": "R1.006", "seats": 50, "requestwith": "NONE",
		"needsRequest": true, "needsrequest": true,
	})
	r, _ := convertRoom(d)
	if r.NeedsRequest {
		t.Error("NeedsRequest must not be carried over -- it is a generated column")
	}
	if left := d.leftovers(); len(left) != 0 {
		t.Errorf("both spellings must count as consumed, leftovers = %v", left)
	}
}

// An unknown key must show up, so nothing disappears unnoticed.
func TestLeftoversReportUnknownKeys(t *testing.T) {
	d := decode(t, bson.M{"name": "R1.006", "seats": 10, "requestwith": "NONE", "hitzegrad": 3})
	convertRoom(d)
	left := d.leftovers()
	if len(left) != 1 || left[0] != "hitzegrad" {
		t.Errorf("leftovers = %v, want [hitzegrad]", left)
	}
}

// int32 is what the driver produces for a plain BSON number.
func TestNumbersDecodeFromInt32(t *testing.T) {
	d := decode(t, bson.M{"name": "R1.006", "seats": 42, "requestwith": "NONE"})
	r, _ := convertRoom(d)
	if r.Seats != 42 {
		t.Errorf("Seats = %d, want 42", r.Seats)
	}
}
