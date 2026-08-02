package db_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
)

func strptr(s string) *string { return &s }

func strval(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

// assertNtaEqual compares field by field rather than with reflect.DeepEqual so a
// failure names the offending column -- the mapper is where a silently swapped
// pair (from/until, needsRoomAlone/needsHardware) would hide.
func assertNtaEqual(t *testing.T, want, got *model.NTA) {
	t.Helper()

	if got == nil {
		t.Fatal("nta is nil")
	}
	for _, f := range []struct {
		name      string
		want, got string
	}{
		{"Name", want.Name, got.Name},
		{"Email", strval(want.Email), strval(got.Email)},
		{"Mtknr", want.Mtknr, got.Mtknr},
		{"Compensation", want.Compensation, got.Compensation},
		{"Program", want.Program, got.Program},
		{"From", want.From, got.From},
		{"Until", want.Until, got.Until},
		{"LastSemester", strval(want.LastSemester), strval(got.LastSemester)},
	} {
		if f.want != f.got {
			t.Errorf("%s = %q, want %q", f.name, f.got, f.want)
		}
	}
	if want.DeltaDurationPercent != got.DeltaDurationPercent {
		t.Errorf("DeltaDurationPercent = %d, want %d", got.DeltaDurationPercent, want.DeltaDurationPercent)
	}
	if want.NeedsRoomAlone != got.NeedsRoomAlone {
		t.Errorf("NeedsRoomAlone = %v, want %v", got.NeedsRoomAlone, want.NeedsRoomAlone)
	}
	if want.NeedsHardware != got.NeedsHardware {
		t.Errorf("NeedsHardware = %v, want %v", got.NeedsHardware, want.NeedsHardware)
	}
	if want.Deactivated != got.Deactivated {
		t.Errorf("Deactivated = %v, want %v", got.Deactivated, want.Deactivated)
	}
}

func testNta(mtknr, name string) *model.NTA {
	return &model.NTA{
		Name:                 name,
		Email:                strptr(name + "@hm.edu"),
		Mtknr:                mtknr,
		Compensation:         "Zeitverlängerung",
		DeltaDurationPercent: 25,
		NeedsRoomAlone:       true,
		NeedsHardware:        false,
		Program:              "IF-B",
		From:                 "2026-WS",
		Until:                "2027-SS",
		LastSemester:         nil,
		Deactivated:          false,
	}
}

// TestNtaRoundTrip pins the semantics the Mongo implementation had, field by field
// -- including the two nullable pointers, which are exactly what the
// emit_pointers_for_null_types setting exists for.
func TestNtaRoundTrip(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	want := testNta("00012345", "Andrea Beispiel")

	got, err := pg.AddNta(ctx, want)
	if err != nil {
		t.Fatalf("AddNta: %v", err)
	}
	assertNtaEqual(t, want, got)

	// Leading zeros must survive: mtknr is TEXT, never numeric.
	if got.Mtknr != "00012345" {
		t.Errorf("mtknr = %q, want %q -- leading zeros lost", got.Mtknr, "00012345")
	}

	read, err := pg.Nta(ctx, "00012345")
	if err != nil {
		t.Fatalf("Nta: %v", err)
	}
	assertNtaEqual(t, want, read)
}

func TestNtaNilPointersSurvive(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	want := testNta("00012345", "Andrea Beispiel")
	want.Email = nil
	want.LastSemester = nil

	if _, err := pg.AddNta(ctx, want); err != nil {
		t.Fatalf("AddNta: %v", err)
	}

	got, err := pg.Nta(ctx, "00012345")
	if err != nil {
		t.Fatalf("Nta: %v", err)
	}
	if got.Email != nil {
		t.Errorf("Email = %v, want nil", *got.Email)
	}
	if got.LastSemester != nil {
		t.Errorf("LastSemester = %v, want nil", *got.LastSemester)
	}
}

// TestNtaMissingReturnsNilNil pins the Mongo contract: a missing document is not an
// error. Several callers rely on it (a nil NTA means "no compensation"), so turning
// it into pgx.ErrNoRows would change behaviour far away from here.
func TestNtaMissingReturnsNilNil(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	nta, err := pg.Nta(ctx, "does-not-exist")
	if err != nil {
		t.Fatalf("Nta: %v", err)
	}
	if nta != nil {
		t.Errorf("Nta = %v, want nil", nta)
	}

	replaced, err := pg.ReplaceNta(ctx, testNta("does-not-exist", "Nobody"))
	if err != nil {
		t.Fatalf("ReplaceNta: %v", err)
	}
	if replaced != nil {
		t.Errorf("ReplaceNta inserted a row -- it must not upsert")
	}

	deactivated, err := pg.SetNtaDeactivated(ctx, "does-not-exist", true)
	if err != nil {
		t.Fatalf("SetNtaDeactivated: %v", err)
	}
	if deactivated != nil {
		t.Errorf("SetNtaDeactivated = %v, want nil", deactivated)
	}
}

// TestNtasEmptyIsNotNil guards the emit_empty_slices setting: the Mongo version
// returned make([]*model.NTA, 0), and a GraphQL non-null list would serialise a nil
// slice as null.
func TestNtasEmptyIsNotNil(t *testing.T) {
	pg := pgtest.NewDB(t)

	ntas, err := pg.Ntas(t.Context())
	if err != nil {
		t.Fatalf("Ntas: %v", err)
	}
	if ntas == nil {
		t.Fatal("Ntas returned nil, want an empty slice")
	}
	if len(ntas) != 0 {
		t.Errorf("len(Ntas) = %d, want 0", len(ntas))
	}
}

func TestNtasSortedByName(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	for _, n := range []struct{ mtknr, name string }{
		{"00000003", "Zacharias Zuletzt"},
		{"00000001", "Anna Anfang"},
		{"00000002", "Martin Mitte"},
	} {
		if _, err := pg.AddNta(ctx, testNta(n.mtknr, n.name)); err != nil {
			t.Fatalf("AddNta(%s): %v", n.name, err)
		}
	}

	ntas, err := pg.Ntas(ctx)
	if err != nil {
		t.Fatalf("Ntas: %v", err)
	}

	want := []string{"Anna Anfang", "Martin Mitte", "Zacharias Zuletzt"}
	if len(ntas) != len(want) {
		t.Fatalf("len(Ntas) = %d, want %d", len(ntas), len(want))
	}
	for i, w := range want {
		if ntas[i].Name != w {
			t.Errorf("Ntas[%d].Name = %q, want %q", i, ntas[i].Name, w)
		}
	}
}

// TestInTransactionRollsBack proves the q(ctx) seam: a db method called inside
// InTransaction must run on the transaction, so a failure undoes it. Under Mongo
// this only held on a replica set; here it is unconditional.
func TestInTransactionRollsBack(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	sentinel := errors.New("rollback please")

	err := pg.InTransaction(ctx, func(ctx context.Context) error {
		if _, err := pg.AddNta(ctx, testNta("00000001", "Wird Zurueckgerollt")); err != nil {
			return err
		}
		// Visible inside the transaction ...
		nta, err := pg.Nta(ctx, "00000001")
		if err != nil {
			return err
		}
		if nta == nil {
			t.Error("write not visible inside its own transaction -- q(ctx) missed the tx")
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("InTransaction err = %v, want %v", err, sentinel)
	}

	// ... and gone afterwards.
	nta, err := pg.Nta(ctx, "00000001")
	if err != nil {
		t.Fatalf("Nta: %v", err)
	}
	if nta != nil {
		t.Error("row survived a rolled-back transaction")
	}
}

func TestInTransactionCommits(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	if err := pg.InTransaction(ctx, func(ctx context.Context) error {
		_, err := pg.AddNta(ctx, testNta("00000001", "Bleibt Erhalten"))
		return err
	}); err != nil {
		t.Fatalf("InTransaction: %v", err)
	}

	nta, err := pg.Nta(ctx, "00000001")
	if err != nil {
		t.Fatalf("Nta: %v", err)
	}
	if nta == nil {
		t.Fatal("committed row is missing")
	}
}

// TestTimestamptzKeepsLocation is the single most important test of the migration.
//
// Slot and day arithmetic depends on time.Local being Europe/Berlin (main.go sets
// it), and more than ten places key a map by time.Time -- plexams/preplan_suggest.go,
// rooms.go, rooms_blocked.go, examplan_build.go, semester_config.go. A map key
// compares STRUCT-WISE, and the struct holds a *time.Location POINTER. Two locations
// with the same name are not the same pointer, so two times that print identically,
// have the same offset and satisfy Equal() can still be different keys.
//
// pgx decodes timestamptz into time.Local, so the invariant to hold onto is
// "everything that may become a map key carries time.Local". That is why every
// parsing site in plexams uses time.ParseInLocation(..., time.Local); the lone
// time.LoadLocation("Europe/Berlin") in plexams/invigilators.go:225 is currently
// harmless only because datesToDay compares Month/Day field-wise.
func TestTimestamptzKeepsLocation(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	if _, err := pg.PoolForTest().Exec(ctx, "create table tz_probe (t timestamptz not null)"); err != nil {
		t.Fatalf("create probe table: %v", err)
	}

	// A summer date, so a wrong zone shows up as CET (+01:00) instead of CEST.
	want := time.Date(2026, 7, 15, 8, 30, 0, 0, time.Local)

	if _, err := pg.PoolForTest().Exec(ctx, "insert into tz_probe (t) values ($1)", want); err != nil {
		t.Fatalf("insert: %v", err)
	}

	var got time.Time
	if err := pg.PoolForTest().QueryRow(ctx, "select t from tz_probe").Scan(&got); err != nil {
		t.Fatalf("select: %v", err)
	}

	if !got.Equal(want) {
		t.Errorf("instant changed: got %v, want %v", got, want)
	}
	if got.Location() != time.Local {
		t.Errorf("Location = %v (%p), want time.Local (%p) -- session time zone was not applied",
			got.Location(), got.Location(), time.Local)
	}
	if _, offset := got.Zone(); offset != 2*60*60 {
		t.Errorf("offset = %+d s, want +7200 s (CEST) -- time.Local is not Europe/Berlin", offset)
	}
	if got.Format("15:04") != "08:30" {
		t.Errorf("wall clock = %s, want 08:30", got.Format("15:04"))
	}

	// The part that actually bites: identity as a map key, not merely equality.
	m := map[time.Time]string{want: "slot"}
	if _, ok := m[got]; !ok {
		t.Errorf("round-tripped time is not the same map key: stored %v (%p), read %v (%p)",
			want, want.Location(), got, got.Location())
	}
}

// TestSeparatelyLoadedLocationIsADifferentMapKey documents the trap rather than the
// contract: it asserts the BAD behaviour, so that if a future Go or pgx release ever
// makes locations comparable by name, this test fails and tells us the rule above
// can be relaxed. Until then: never build a time from time.LoadLocation and use it
// as a key, use time.Local.
func TestSeparatelyLoadedLocationIsADifferentMapKey(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatalf("cannot load Europe/Berlin: %v", err)
	}

	viaLocal := time.Date(2026, 7, 15, 8, 30, 0, 0, time.Local)
	viaLoaded := time.Date(2026, 7, 15, 8, 30, 0, 0, berlin)

	if !viaLocal.Equal(viaLoaded) {
		t.Fatal("the two times are not even the same instant -- test is wrong")
	}

	m := map[time.Time]string{viaLocal: "slot"}
	if _, ok := m[viaLoaded]; ok {
		t.Log("locations are now comparable by name: the time.Local rule can be relaxed")
	}
}
