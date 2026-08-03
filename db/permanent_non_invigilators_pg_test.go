package db_test

import (
	"testing"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
)

func testNonInvigilator(teacherID int, name string) *model.PermanentNonInvigilator {
	return &model.PermanentNonInvigilator{
		TeacherID:  teacherID,
		Name:       name,
		Reason:     "pensioniert",
		ValidFrom:  nil,
		ValidUntil: nil,
	}
}

// TestPermanentNonInvigilatorRoundTrip covers the shape of all ten live entries:
// teacherID, a denormalized name and a reason, and no validity window at all --
// the fields exist for retiring an exemption without deleting the history.
func TestPermanentNonInvigilatorRoundTrip(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	want := testNonInvigilator(298, "Prof. Dr. Gerhard Stützle")
	if err := pg.UpsertPermanentNonInvigilator(ctx, want); err != nil {
		t.Fatalf("UpsertPermanentNonInvigilator: %v", err)
	}

	got, err := pg.PermanentNonInvigilators(ctx)
	if err != nil {
		t.Fatalf("PermanentNonInvigilators: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].TeacherID != want.TeacherID {
		t.Errorf("TeacherID = %d, want %d", got[0].TeacherID, want.TeacherID)
	}
	if got[0].Name != want.Name {
		t.Errorf("Name = %q, want %q", got[0].Name, want.Name)
	}
	if got[0].Reason != want.Reason {
		t.Errorf("Reason = %q, want %q", got[0].Reason, want.Reason)
	}
	if got[0].ValidFrom != nil || got[0].ValidUntil != nil {
		t.Errorf("validity = %s..%s, want nil..nil",
			strval(got[0].ValidFrom), strval(got[0].ValidUntil))
	}
}

func TestPermanentNonInvigilatorValidityWindow(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	want := testNonInvigilator(7313, "Prof. Dr. Maike Grabinski")
	want.Reason = "Mutterschutz"
	want.ValidFrom = strptr("2026-WS")
	want.ValidUntil = strptr("2027-SS")

	if err := pg.UpsertPermanentNonInvigilator(ctx, want); err != nil {
		t.Fatalf("UpsertPermanentNonInvigilator: %v", err)
	}

	got, err := pg.PermanentNonInvigilators(ctx)
	if err != nil {
		t.Fatalf("PermanentNonInvigilators: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if strval(got[0].ValidFrom) != "2026-WS" || strval(got[0].ValidUntil) != "2027-SS" {
		t.Errorf("validity = %s..%s, want 2026-WS..2027-SS",
			strval(got[0].ValidFrom), strval(got[0].ValidUntil))
	}
}

func TestPermanentNonInvigilatorUpsertReplaces(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	if err := pg.UpsertPermanentNonInvigilator(ctx, testNonInvigilator(217, "Prof. Dr. Jochen Hertle")); err != nil {
		t.Fatalf("UpsertPermanentNonInvigilator: %v", err)
	}

	updated := testNonInvigilator(217, "Prof. Dr. Jochen Hertle")
	updated.Reason = "Dekan"
	if err := pg.UpsertPermanentNonInvigilator(ctx, updated); err != nil {
		t.Fatalf("UpsertPermanentNonInvigilator (second): %v", err)
	}

	got, err := pg.PermanentNonInvigilators(ctx)
	if err != nil {
		t.Fatalf("PermanentNonInvigilators: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 -- the upsert inserted instead of replacing", len(got))
	}
	if got[0].Reason != "Dekan" {
		t.Errorf("Reason = %q, want %q", got[0].Reason, "Dekan")
	}
}

func TestPermanentNonInvigilatorDelete(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	deleted, err := pg.DeletePermanentNonInvigilator(ctx, 217)
	if err != nil {
		t.Fatalf("DeletePermanentNonInvigilator: %v", err)
	}
	if deleted {
		t.Error("DeletePermanentNonInvigilator = true for an entry that does not exist")
	}

	if err := pg.UpsertPermanentNonInvigilator(ctx, testNonInvigilator(217, "Prof. Dr. Jochen Hertle")); err != nil {
		t.Fatalf("UpsertPermanentNonInvigilator: %v", err)
	}

	deleted, err = pg.DeletePermanentNonInvigilator(ctx, 217)
	if err != nil {
		t.Fatalf("DeletePermanentNonInvigilator: %v", err)
	}
	if !deleted {
		t.Error("DeletePermanentNonInvigilator = false for an entry that exists")
	}
}

// TestPermanentNonInvigilatorsEmptyIsNotNil matters here in particular: the
// invigilator code subtracts this list from the candidate pool, and a nil slice
// would serialise as null in a GraphQL non-null list.
func TestPermanentNonInvigilatorsEmptyIsNotNil(t *testing.T) {
	pg := pgtest.NewDB(t)

	got, err := pg.PermanentNonInvigilators(t.Context())
	if err != nil {
		t.Fatalf("PermanentNonInvigilators: %v", err)
	}
	if got == nil {
		t.Fatal("PermanentNonInvigilators returned nil, want an empty slice")
	}
}

// The Mongo version had no sort at all. Ordering by name is new, and stable.
func TestPermanentNonInvigilatorsSortedByName(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	for _, n := range []struct {
		id   int
		name string
	}{
		{274, "Prof. Dr. Reinhard Schiedermeier"},
		{245, "Prof. Dr. Martin Leitner"},
		{301, "Prof. Dr. Veronika Thurner"},
	} {
		if err := pg.UpsertPermanentNonInvigilator(ctx, testNonInvigilator(n.id, n.name)); err != nil {
			t.Fatalf("UpsertPermanentNonInvigilator(%d): %v", n.id, err)
		}
	}

	got, err := pg.PermanentNonInvigilators(ctx)
	if err != nil {
		t.Fatalf("PermanentNonInvigilators: %v", err)
	}
	want := []string{
		"Prof. Dr. Martin Leitner",
		"Prof. Dr. Reinhard Schiedermeier",
		"Prof. Dr. Veronika Thurner",
	}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].Name != w {
			t.Errorf("[%d].Name = %q, want %q", i, got[i].Name, w)
		}
	}
}
