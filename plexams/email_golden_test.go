package plexams

import (
	"bytes"
	"flag"
	htmltmpl "html/template"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	txttmpl "text/template"
)

var updateGolden = flag.Bool("update", false, "update the email golden files")

// parseFuncs is the superset of template funcs used across all email templates: the
// global emailFuncs plus the few registered ad hoc at some call sites (e.g. "add" for the
// assembled-exam markdown). Used only to parse every template in the guard below.
func parseFuncs() map[string]any {
	fns := map[string]any{"add": func(a, b int) int { return a + b }}
	for k, v := range emailFuncs {
		fns[k] = v
	}
	return fns
}

// TestAllEmailTemplatesParse is a cheap global guard: every embedded email template must
// parse. It catches a syntax break in any template during the templates refactor, without
// needing per-template fixture data. HTML templates are parsed with html/template, the
// rest with text/template, matching how they are used.
func TestAllEmailTemplatesParse(t *testing.T) {
	entries, err := fs.ReadDir(emailTemplates, "tmpl")
	if err != nil {
		t.Fatalf("read embedded templates: %v", err)
	}
	funcs := parseFuncs()
	n := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".tmpl") {
			continue
		}
		n++
		path := "tmpl/" + name
		if strings.HasSuffix(name, "HTML.tmpl") {
			if _, err := htmltmpl.New(name).Funcs(htmltmpl.FuncMap(funcs)).ParseFS(emailTemplates, path); err != nil {
				t.Errorf("html template %s does not parse: %v", name, err)
			}
			continue
		}
		if _, err := txttmpl.New(name).Funcs(txttmpl.FuncMap(funcs)).ParseFS(emailTemplates, path); err != nil {
			t.Errorf("text template %s does not parse: %v", name, err)
		}
	}
	if n < 40 {
		t.Errorf("expected ~47 email templates, only found %d", n)
	}
	t.Logf("parsed %d email templates", n)
}

// assertGolden compares got against testdata/email/<name>; with -update it (re)writes the
// golden. Used to lock an email's rendered output before/through the templates refactor.
func assertGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", "email", name)
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read golden %s (run: go test -run <Test> -update): %v", path, err)
	}
	if !bytes.Equal(want, got) {
		t.Errorf("%s differs from golden (run -update to refresh and inspect the diff)", name)
	}
}

// renderTextTemplate renders one text email template with data (test helper).
func renderTextTemplate(t *testing.T, name string, data any) []byte {
	t.Helper()
	tmpl, err := txttmpl.New(name).Funcs(txttmpl.FuncMap(emailFuncs)).ParseFS(emailTemplates, "tmpl/"+name)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("execute %s: %v", name, err)
	}
	return buf.Bytes()
}

// TestExahmEmailGolden locks the EXaHM/SEB request email (text + HTML) against a golden.
// It is the first render-golden; more are added as each email is migrated to Markdown.
func TestExahmEmailGolden(t *testing.T) {
	data := &ExahmEmail{PlanerName: "Test Planer"}

	text := renderTextTemplate(t, "exahmEmail.tmpl", data)
	assertGolden(t, "exahmEmail.txt", text)

	html, err := (&Plexams{}).renderMailHTML("tmpl/exahmEmailHTML.tmpl", true, data)
	if err != nil {
		t.Fatalf("render html: %v", err)
	}
	assertGolden(t, "exahmEmail.html", html)
}
