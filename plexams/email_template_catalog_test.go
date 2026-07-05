package plexams

import (
	"strings"
	"testing"

	"github.com/obcode/plexams.go/plexams/email"
)

// TestEmailTemplateCatalogPreviewRenders is the guard for the GUI catalog: every editable
// template must (a) have a catalog entry and (b) render cleanly against its sample data —
// both its built-in default source and via the preview path. This catches a sample-data
// shape that does not match a template's field accesses, which would otherwise only surface
// at runtime in the GUI preview.
func TestEmailTemplateCatalogPreviewRenders(t *testing.T) {
	names, err := email.TemplateNames()
	if err != nil {
		t.Fatal(err)
	}
	// store nil -> only embedded defaults; no DB needed.
	r := email.New(nil, renderFuncs(), jiraURL)

	for _, name := range names {
		info, ok := emailTemplateCatalog[name]
		if !ok {
			t.Errorf("template %q has no catalog entry (description/variables/sample)", name)
			continue
		}
		if info.Description == "" {
			t.Errorf("template %q has empty catalog description", name)
		}
		if len(info.Variables) == 0 {
			t.Errorf("template %q has no documented variables", name)
		}
		src, err := email.EmbeddedSource(name)
		if err != nil {
			t.Errorf("template %q: cannot read embedded source: %v", name, err)
			continue
		}
		text, html, err := r.RenderSource(name, src, info.Jira, info.Sample)
		if err != nil {
			t.Errorf("template %q does not render against its sample data: %v", name, err)
			continue
		}
		if strings.Contains(string(text), "<no value>") {
			t.Errorf("template %q preview text has an unfilled placeholder (<no value>)", name)
		}
		if len(html) == 0 {
			t.Errorf("template %q rendered empty HTML", name)
		}
	}

	// Every catalog key must correspond to a real template (no stale entries).
	known := make(map[string]bool, len(names))
	for _, n := range names {
		known[n] = true
	}
	for name := range emailTemplateCatalog {
		if !known[name] {
			t.Errorf("catalog has entry for unknown template %q", name)
		}
	}
}
