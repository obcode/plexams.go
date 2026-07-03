package plexams

import (
	"context"
	"strings"
	"testing"

	"github.com/obcode/plexams.go/internal/mongotest"
)

// TestEmailTemplateOverrideRoundTrip exercises the DB-backed, editable email templates
// against a real MongoDB: default → set override (used at render) → validate → reset.
func TestEmailTemplateOverrideRoundTrip(t *testing.T) {
	p := &Plexams{dbClient: mongotest.NewDB(t)}
	ctx := context.Background()
	const name = "exahmEmail.md.tmpl"

	// default: no override, render uses the embedded template.
	text, _, err := p.mailRenderer().Render(name, true, &ExahmEmail{PlanerName: "X"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(text), "der Ansturm auf die Räume") {
		t.Errorf("default render missing expected content")
	}

	// set an override; the render must use it.
	if _, err := p.SetEmailTemplate(ctx, name, "Hallo Welt {{ .PlanerName }}"); err != nil {
		t.Fatalf("set override: %v", err)
	}
	text, _, err = p.mailRenderer().Render(name, true, &ExahmEmail{PlanerName: "Käthe"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(text), "Hallo Welt Käthe") {
		t.Errorf("override not used at render, got: %q", string(text))
	}

	// the list reflects the override.
	tmpls, err := p.EmailTemplates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tm := range tmpls {
		if tm.Name == name {
			found = true
			if tm.IsDefault || !strings.Contains(tm.Markdown, "Hallo Welt") {
				t.Errorf("list wrong for overridden template: %+v", tm)
			}
			if !strings.Contains(tm.DefaultMarkdown, "der Ansturm") {
				t.Errorf("defaultMarkdown not exposed")
			}
		}
	}
	if !found {
		t.Fatalf("template %s not in list", name)
	}

	// invalid Markdown (broken template) is rejected.
	if _, err := p.SetEmailTemplate(ctx, name, "{{ .Foo "); err == nil {
		t.Errorf("expected parse error for broken template")
	}
	// unknown template name is rejected.
	if _, err := p.SetEmailTemplate(ctx, "does-not-exist.md.tmpl", "x"); err == nil {
		t.Errorf("expected error for unknown template")
	}

	// reset reverts to the embedded default.
	ok, err := p.ResetEmailTemplate(ctx, name)
	if err != nil || !ok {
		t.Fatalf("reset: ok=%v err=%v", ok, err)
	}
	text, _, err = p.mailRenderer().Render(name, true, &ExahmEmail{PlanerName: "X"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(text), "der Ansturm auf die Räume") {
		t.Errorf("render did not revert to default after reset")
	}
}
