package db_test

import (
	"testing"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
)

func TestAnnyConfigRoundTrip(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	want := &model.AnnyConfig{
		PersonalizationNames: []string{"Prüfungsplanung FK07", "Braun, Oliver"},
	}
	if err := pg.SetAnnyConfig(ctx, want); err != nil {
		t.Fatalf("SetAnnyConfig: %v", err)
	}

	got, err := pg.GetAnnyConfig(ctx)
	if err != nil {
		t.Fatalf("GetAnnyConfig: %v", err)
	}
	if got == nil {
		t.Fatal("anny config is nil")
	}
	if len(got.PersonalizationNames) != len(want.PersonalizationNames) {
		t.Fatalf("len = %d, want %d", len(got.PersonalizationNames), len(want.PersonalizationNames))
	}
	for i, w := range want.PersonalizationNames {
		if got.PersonalizationNames[i] != w {
			t.Errorf("PersonalizationNames[%d] = %q, want %q", i, got.PersonalizationNames[i], w)
		}
	}
}

func TestAnnyConfigMissingReturnsNilNil(t *testing.T) {
	pg := pgtest.NewDB(t)

	got, err := pg.GetAnnyConfig(t.Context())
	if err != nil {
		t.Fatalf("GetAnnyConfig: %v", err)
	}
	if got != nil {
		t.Errorf("GetAnnyConfig = %v, want nil", got)
	}
}

// personalizationNames is [String!]! in the schema: an empty list has to stay an
// empty list all the way through, because a nil slice serialises to null and the
// GUI would show nothing rather than "no names configured".
func TestAnnyConfigEmptyNamesStayEmpty(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	for _, names := range [][]string{{}, nil} {
		if err := pg.SetAnnyConfig(ctx, &model.AnnyConfig{PersonalizationNames: names}); err != nil {
			t.Fatalf("SetAnnyConfig(%v): %v", names, err)
		}

		got, err := pg.GetAnnyConfig(ctx)
		if err != nil {
			t.Fatalf("GetAnnyConfig: %v", err)
		}
		if got == nil {
			t.Fatal("anny config is nil")
		}
		if got.PersonalizationNames == nil {
			t.Errorf("PersonalizationNames is nil after storing %v, want an empty slice", names)
		}
		if len(got.PersonalizationNames) != 0 {
			t.Errorf("len = %d, want 0", len(got.PersonalizationNames))
		}
	}
}

func TestAnnyConfigIsASingleton(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	if err := pg.SetAnnyConfig(ctx, &model.AnnyConfig{PersonalizationNames: []string{"erster"}}); err != nil {
		t.Fatalf("SetAnnyConfig: %v", err)
	}
	if err := pg.SetAnnyConfig(ctx, &model.AnnyConfig{PersonalizationNames: []string{"zweiter"}}); err != nil {
		t.Fatalf("SetAnnyConfig (second): %v", err)
	}

	if n := count(t, pg, "select count(*) from anny_config"); n != 1 {
		t.Errorf("anny_config rows = %d, want 1", n)
	}
	got, err := pg.GetAnnyConfig(ctx)
	if err != nil {
		t.Fatalf("GetAnnyConfig: %v", err)
	}
	if len(got.PersonalizationNames) != 1 || got.PersonalizationNames[0] != "zweiter" {
		t.Errorf("PersonalizationNames = %v, want [zweiter]", got.PersonalizationNames)
	}
}
