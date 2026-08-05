package db_test

import (
	"testing"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/internal/pgtest"
)

// registerSemesters registers the semesters a planner override can hang off.
// The foreign key rejects an override for an unregistered semester, which is the
// point of TestSemesterPlanerNeedsARegisteredSemester below.
func registerSemesters(t *testing.T, pg *db.PG, ids ...string) {
	t.Helper()
	for _, id := range ids {
		if err := pg.EnsureSemester(t.Context(), id, db.CurrentSchemaVersion); err != nil {
			t.Fatalf("EnsureSemester(%q): %v", id, err)
		}
	}
}

func TestSemesterPlanerRoundTrip(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()
	registerSemesters(t, pg, pg.Semester())

	want := &db.SemesterPlaner{
		Name:        strptr("Oliver Braun"),
		Email:       strptr("oliver.braun@hm.edu"),
		TestMail:    strptr("oliver.braun+test@hm.edu"),
		Cc:          strptr("pruefungsplanung@hm.edu"),
		NoreplyMail: strptr("noreply+plexams@hm.edu"),
		NoreplyName: strptr("Prüfungsplanung FK07 (NOREPLY)"),
	}

	if err := pg.SaveSemesterPlaner(ctx, want); err != nil {
		t.Fatalf("SaveSemesterPlaner: %v", err)
	}

	got, err := pg.GetSemesterPlaner(ctx)
	if err != nil {
		t.Fatalf("GetSemesterPlaner: %v", err)
	}
	if got == nil {
		t.Fatal("semester planer is nil")
	}
	for _, f := range []struct {
		name      string
		want, got *string
	}{
		{"Name", want.Name, got.Name},
		{"Email", want.Email, got.Email},
		{"TestMail", want.TestMail, got.TestMail},
		{"Cc", want.Cc, got.Cc},
		{"NoreplyMail", want.NoreplyMail, got.NoreplyMail},
		{"NoreplyName", want.NoreplyName, got.NoreplyName},
	} {
		if strval(f.want) != strval(f.got) {
			t.Errorf("%s = %q, want %q", f.name, strval(f.got), strval(f.want))
		}
	}
}

// A semester without a row inherits from the server config, and nil/nil is how
// the db layer says so -- not an empty struct, which plexams could not tell apart
// from an override that blanks everything.
func TestSemesterPlanerMissingReturnsNilNil(t *testing.T) {
	pg := pgtest.NewDB(t)
	registerSemesters(t, pg, pg.Semester())

	got, err := pg.GetSemesterPlaner(t.Context())
	if err != nil {
		t.Fatalf("GetSemesterPlaner: %v", err)
	}
	if got != nil {
		t.Errorf("GetSemesterPlaner = %v, want nil", got)
	}
}

// Only the sender overrides, no own identity: a semester may keep the configured
// planner but send its dry runs somewhere else.
func TestSemesterPlanerSenderOverridesWithoutIdentity(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()
	registerSemesters(t, pg, pg.Semester())

	if err := pg.SaveSemesterPlaner(ctx, &db.SemesterPlaner{
		TestMail: strptr("probelauf@hm.edu"),
	}); err != nil {
		t.Fatalf("SaveSemesterPlaner: %v", err)
	}

	got, err := pg.GetSemesterPlaner(ctx)
	if err != nil {
		t.Fatalf("GetSemesterPlaner: %v", err)
	}
	if got.Name != nil || got.Email != nil {
		t.Errorf("identity = %q/%q, want both nil (inherited)", strval(got.Name), strval(got.Email))
	}
	if strval(got.TestMail) != "probelauf@hm.edu" {
		t.Errorf("TestMail = %q, want %q", strval(got.TestMail), "probelauf@hm.edu")
	}
}

// Name and email are one identity. Half of one would send as "Oliver Braun
// <someone.else@hm.edu>", so the table refuses it rather than trusting every
// caller to check.
func TestSemesterPlanerIdentityIsAllOrNothing(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()
	registerSemesters(t, pg, pg.Semester())

	for _, tc := range []struct {
		name   string
		planer *db.SemesterPlaner
	}{
		{"name without email", &db.SemesterPlaner{Name: strptr("Oliver Braun")}},
		{"email without name", &db.SemesterPlaner{Email: strptr("oliver.braun@hm.edu")}},
	} {
		if err := pg.SaveSemesterPlaner(ctx, tc.planer); err == nil {
			t.Errorf("%s was accepted", tc.name)
		}
	}
}

// The whole point of the change: two semesters, two planners, no interference.
func TestSemesterPlanerIsPerSemester(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()
	first := pg.Semester()
	registerSemesters(t, pg, first, "2027-SS")

	if err := pg.SaveSemesterPlaner(ctx, &db.SemesterPlaner{
		Name: strptr("Erster"), Email: strptr("erster@hm.edu"),
	}); err != nil {
		t.Fatalf("SaveSemesterPlaner: %v", err)
	}

	pg.SwitchTo(ctx, "2027-SS")
	got, err := pg.GetSemesterPlaner(ctx)
	if err != nil {
		t.Fatalf("GetSemesterPlaner: %v", err)
	}
	if got != nil {
		t.Fatalf("2027-SS = %v, want nil -- the other semester's planner leaked", got)
	}

	if err := pg.SaveSemesterPlaner(ctx, &db.SemesterPlaner{
		Name: strptr("Zweiter"), Email: strptr("zweiter@hm.edu"),
	}); err != nil {
		t.Fatalf("SaveSemesterPlaner: %v", err)
	}

	pg.SwitchTo(ctx, first)
	got, err = pg.GetSemesterPlaner(ctx)
	if err != nil {
		t.Fatalf("GetSemesterPlaner: %v", err)
	}
	if strval(got.Name) != "Erster" {
		t.Errorf("Name = %q, want %q", strval(got.Name), "Erster")
	}
}

// Deleting restores inheritance, and deleting nothing is not an error -- the GUI
// reset button must work on a semester that never had an override.
func TestDeleteSemesterPlaner(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()
	registerSemesters(t, pg, pg.Semester())

	if err := pg.DeleteSemesterPlaner(ctx); err != nil {
		t.Fatalf("DeleteSemesterPlaner without a row: %v", err)
	}

	if err := pg.SaveSemesterPlaner(ctx, &db.SemesterPlaner{
		Name: strptr("Oliver Braun"), Email: strptr("oliver.braun@hm.edu"),
	}); err != nil {
		t.Fatalf("SaveSemesterPlaner: %v", err)
	}
	if err := pg.DeleteSemesterPlaner(ctx); err != nil {
		t.Fatalf("DeleteSemesterPlaner: %v", err)
	}

	got, err := pg.GetSemesterPlaner(ctx)
	if err != nil {
		t.Fatalf("GetSemesterPlaner: %v", err)
	}
	if got != nil {
		t.Errorf("GetSemesterPlaner = %v, want nil after delete", got)
	}
}

func TestSemesterPlanerNeedsARegisteredSemester(t *testing.T) {
	pg := pgtest.NewDB(t)

	if err := pg.SaveSemesterPlaner(t.Context(), &db.SemesterPlaner{
		Name: strptr("Oliver Braun"), Email: strptr("oliver.braun@hm.edu"),
	}); err == nil {
		t.Error("an override for an unregistered semester was accepted")
	}
}
