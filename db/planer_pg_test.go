package db_test

import (
	"testing"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
)

func TestPlanerRoundTrip(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	want := &model.Planer{
		Name:        "Oliver Braun",
		Email:       "oliver.braun@hm.edu",
		TestMail:    strptr("oliver.braun+test@hm.edu"),
		Cc:          strptr("pruefungsplanung@hm.edu"),
		NoreplyMail: strptr("noreply+plexams@hm.edu"),
		NoreplyName: strptr("Prüfungsplanung FK07 (NOREPLY)"),
	}

	if err := pg.SavePlaner(ctx, want); err != nil {
		t.Fatalf("SavePlaner: %v", err)
	}

	got, err := pg.GetPlaner(ctx)
	if err != nil {
		t.Fatalf("GetPlaner: %v", err)
	}
	if got == nil {
		t.Fatal("planer is nil")
	}
	for _, f := range []struct {
		name      string
		want, got string
	}{
		{"Name", want.Name, got.Name},
		{"Email", want.Email, got.Email},
		{"TestMail", strval(want.TestMail), strval(got.TestMail)},
		{"Cc", strval(want.Cc), strval(got.Cc)},
		{"NoreplyMail", strval(want.NoreplyMail), strval(got.NoreplyMail)},
		{"NoreplyName", strval(want.NoreplyName), strval(got.NoreplyName)},
	} {
		if f.want != f.got {
			t.Errorf("%s = %q, want %q", f.name, f.got, f.want)
		}
	}
}

// TestPlanerLiveShape is the shape the stored document actually has: name and
// email, and not one of the four overrides. They must come back as nil, because
// nil is what makes the derived defaults apply.
func TestPlanerLiveShape(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	if err := pg.SavePlaner(ctx, &model.Planer{
		Name:  "Oliver Braun",
		Email: "oliver.braun@hm.edu",
	}); err != nil {
		t.Fatalf("SavePlaner: %v", err)
	}

	got, err := pg.GetPlaner(ctx)
	if err != nil {
		t.Fatalf("GetPlaner: %v", err)
	}
	if got == nil {
		t.Fatal("planer is nil")
	}
	for _, f := range []struct {
		name string
		got  *string
	}{
		{"TestMail", got.TestMail},
		{"Cc", got.Cc},
		{"NoreplyMail", got.NoreplyMail},
		{"NoreplyName", got.NoreplyName},
	} {
		if f.got != nil {
			t.Errorf("%s = %q, want nil", f.name, *f.got)
		}
	}
}

// TestPlanerDerivedFieldsAreNotStored pins the schema decision. Mongo replaced
// the whole struct, so DefaultMail and the four Effective* values were written
// alongside the overrides and could outlive the value they were derived from.
// Here there are no columns for them, and a caller that hands them in gets them
// back at zero -- plexams recomputes them in planerModel.
func TestPlanerDerivedFieldsAreNotStored(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	if err := pg.SavePlaner(ctx, &model.Planer{
		Name:                 "Oliver Braun",
		Email:                "oliver.braun@hm.edu",
		DefaultMail:          "stale+plexams@hm.edu",
		EffectiveTestMail:    "stale+test@hm.edu",
		EffectiveCc:          "stale+cc@hm.edu",
		EffectiveNoreplyMail: "stale+noreply@hm.edu",
		EffectiveNoreplyName: "Stale",
	}); err != nil {
		t.Fatalf("SavePlaner: %v", err)
	}

	got, err := pg.GetPlaner(ctx)
	if err != nil {
		t.Fatalf("GetPlaner: %v", err)
	}
	for _, f := range []struct {
		name, got string
	}{
		{"DefaultMail", got.DefaultMail},
		{"EffectiveTestMail", got.EffectiveTestMail},
		{"EffectiveCc", got.EffectiveCc},
		{"EffectiveNoreplyMail", got.EffectiveNoreplyMail},
		{"EffectiveNoreplyName", got.EffectiveNoreplyName},
	} {
		if f.got != "" {
			t.Errorf("%s = %q was persisted -- it is derived and must not be stored", f.name, f.got)
		}
	}
}

func TestPlanerMissingReturnsNilNil(t *testing.T) {
	pg := pgtest.NewDB(t)

	got, err := pg.GetPlaner(t.Context())
	if err != nil {
		t.Fatalf("GetPlaner: %v", err)
	}
	if got != nil {
		t.Errorf("GetPlaner = %v, want nil", got)
	}
}

// The planer is a singleton. Under Mongo that was an empty replace filter and a
// second document would simply have been ignored; here the primary key check
// makes a second row impossible.
func TestPlanerIsASingleton(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	if err := pg.SavePlaner(ctx, &model.Planer{Name: "Erster", Email: "erster@hm.edu"}); err != nil {
		t.Fatalf("SavePlaner: %v", err)
	}
	if err := pg.SavePlaner(ctx, &model.Planer{Name: "Zweiter", Email: "zweiter@hm.edu"}); err != nil {
		t.Fatalf("SavePlaner (second): %v", err)
	}

	if n := count(t, pg, "select count(*) from planer"); n != 1 {
		t.Errorf("planer rows = %d, want 1", n)
	}
	got, err := pg.GetPlaner(ctx)
	if err != nil {
		t.Fatalf("GetPlaner: %v", err)
	}
	if got.Name != "Zweiter" {
		t.Errorf("Name = %q, want %q", got.Name, "Zweiter")
	}

	if _, err := pg.PoolForTest().Exec(ctx,
		`insert into planer (id, name, email) values (2, 'Dritter', 'dritter@hm.edu')`); err == nil {
		t.Error("a second planer row was accepted")
	}
}
