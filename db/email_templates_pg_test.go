package db_test

import (
	"testing"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/internal/pgtest"
	"github.com/obcode/plexams.go/plexams/email"
)

// The renderer takes the override store as an interface, so this is the one place
// in 3a where a signature is checked by something other than the call sites. It
// costs nothing and it fails at compile time.
var _ email.TemplateStore = (*db.PG)(nil)

func TestEmailTemplateOverrideRoundTrip(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	// Markdown with the German text and the template actions that actually occur
	// in the built-in templates -- a naive quoting bug would show up here.
	const markdown = "## Prüfungsplan {{ .Semester }}\n\nSehr geehrte:r {{ .Name }},\n\n" +
		"anbei der Plan für **{{ len .Exams }}** Prüfungen.\n"

	if err := pg.SetEmailTemplateOverride(ctx, "plan-published", markdown); err != nil {
		t.Fatalf("SetEmailTemplateOverride: %v", err)
	}

	got, ok, err := pg.EmailTemplateOverride(ctx, "plan-published")
	if err != nil {
		t.Fatalf("EmailTemplateOverride: %v", err)
	}
	if !ok {
		t.Fatal("EmailTemplateOverride reports no override right after one was stored")
	}
	if got != markdown {
		t.Errorf("markdown = %q, want %q", got, markdown)
	}
}

// A missing override is the normal case -- the compiled-in template is the
// default -- so it must be (\"\", false, nil) and never an error.
func TestEmailTemplateOverrideMissing(t *testing.T) {
	pg := pgtest.NewDB(t)

	got, ok, err := pg.EmailTemplateOverride(t.Context(), "does-not-exist")
	if err != nil {
		t.Fatalf("EmailTemplateOverride: %v", err)
	}
	if ok {
		t.Error("EmailTemplateOverride reports an override that was never stored")
	}
	if got != "" {
		t.Errorf("markdown = %q, want the empty string", got)
	}
}

func TestEmailTemplateOverrideReplaces(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	if err := pg.SetEmailTemplateOverride(ctx, "plan-published", "erste Fassung"); err != nil {
		t.Fatalf("SetEmailTemplateOverride: %v", err)
	}
	if err := pg.SetEmailTemplateOverride(ctx, "plan-published", "zweite Fassung"); err != nil {
		t.Fatalf("SetEmailTemplateOverride (second): %v", err)
	}

	overrides, err := pg.EmailTemplateOverrides(ctx)
	if err != nil {
		t.Fatalf("EmailTemplateOverrides: %v", err)
	}
	if len(overrides) != 1 {
		t.Fatalf("len = %d, want 1 -- the upsert inserted instead of replacing", len(overrides))
	}
	if overrides["plan-published"] != "zweite Fassung" {
		t.Errorf("markdown = %q, want %q", overrides["plan-published"], "zweite Fassung")
	}
}

// The renderer iterates the map and falls back to the built-in template per
// name, so an empty database has to yield an empty map, not nil.
func TestEmailTemplateOverridesEmptyIsNotNil(t *testing.T) {
	pg := pgtest.NewDB(t)

	overrides, err := pg.EmailTemplateOverrides(t.Context())
	if err != nil {
		t.Fatalf("EmailTemplateOverrides: %v", err)
	}
	if overrides == nil {
		t.Fatal("EmailTemplateOverrides returned nil, want an empty map")
	}
	if len(overrides) != 0 {
		t.Errorf("len = %d, want 0", len(overrides))
	}
}

func TestEmailTemplateOverridesAll(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	want := map[string]string{
		"plan-published":   "Der Plan steht.",
		"invigilation-ask": "Bitte um Aufsichten.",
		"room-published":   "Die Räume stehen.",
		"nta-confirmation": "Ihr Nachteilsausgleich ist eingetragen.",
	}
	for name, markdown := range want {
		if err := pg.SetEmailTemplateOverride(ctx, name, markdown); err != nil {
			t.Fatalf("SetEmailTemplateOverride(%s): %v", name, err)
		}
	}

	got, err := pg.EmailTemplateOverrides(ctx)
	if err != nil {
		t.Fatalf("EmailTemplateOverrides: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for name, markdown := range want {
		if got[name] != markdown {
			t.Errorf("overrides[%q] = %q, want %q", name, got[name], markdown)
		}
	}
}

func TestEmailTemplateOverrideDelete(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	deleted, err := pg.DeleteEmailTemplateOverride(ctx, "plan-published")
	if err != nil {
		t.Fatalf("DeleteEmailTemplateOverride: %v", err)
	}
	if deleted {
		t.Error("DeleteEmailTemplateOverride = true for an override that does not exist")
	}

	if err := pg.SetEmailTemplateOverride(ctx, "plan-published", "eine Fassung"); err != nil {
		t.Fatalf("SetEmailTemplateOverride: %v", err)
	}

	deleted, err = pg.DeleteEmailTemplateOverride(ctx, "plan-published")
	if err != nil {
		t.Fatalf("DeleteEmailTemplateOverride: %v", err)
	}
	if !deleted {
		t.Error("DeleteEmailTemplateOverride = false for an override that exists")
	}

	_, ok, err := pg.EmailTemplateOverride(ctx, "plan-published")
	if err != nil {
		t.Fatalf("EmailTemplateOverride: %v", err)
	}
	if ok {
		t.Error("the override survived the delete")
	}
}
