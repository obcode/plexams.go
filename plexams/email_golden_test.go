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

	"github.com/obcode/plexams.go/graph/model"
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
	if n < 20 {
		t.Errorf("suspiciously few email templates found (%d) — glob broken?", n)
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

// TestExahmEmailGolden locks the EXaHM/SEB request email (text + HTML) against a golden,
// rendered through the production Markdown single-source path. First migrated email; the
// rest follow the same pattern.
func TestExahmEmailGolden(t *testing.T) {
	data := &ExahmEmail{PlanerName: "Test Planer"}

	text, html, err := (&Plexams{}).renderMarkdownEmail("exahmEmail.md.tmpl", true, data)
	if err != nil {
		t.Fatalf("render markdown email: %v", err)
	}
	assertGolden(t, "exahmEmail.txt", text)
	assertGolden(t, "exahmEmail.html", html)
}

// TestDraftEmailsGolden locks the draft-plan emails (ZPA, with JIRA; FS, without).
func TestDraftEmailsGolden(t *testing.T) {
	data := &ConstraintsEmail{FromDate: "06.07.26", UntilDate: "17.07.26", FeedbackDate: "13.07.26", PlanerName: "Test Planer"}

	textZ, htmlZ, err := (&Plexams{}).renderMarkdownEmail("draftEmailZPA.md.tmpl", true, data)
	if err != nil {
		t.Fatalf("render ZPA: %v", err)
	}
	assertGolden(t, "draftEmailZPA.txt", textZ)
	assertGolden(t, "draftEmailZPA.html", htmlZ)

	textF, htmlF, err := (&Plexams{}).renderMarkdownEmail("draftEmailFS.md.tmpl", false, data)
	if err != nil {
		t.Fatalf("render FS: %v", err)
	}
	assertGolden(t, "draftEmailFS.txt", textF)
	assertGolden(t, "draftEmailFS.html", htmlF)
}

// TestSimpleEmailsGolden locks the simple single-recipient emails migrated in batch 2.
func TestSimpleEmailsGolden(t *testing.T) {
	teacher := &model.Teacher{Fullname: "Prof. Test"}

	cases := []struct {
		name string
		tmpl string
		jira bool
		data any
	}{
		{"coverPageEmail", "coverPageEmail.md.tmpl", false,
			&CoverMailData{Teacher: teacher, PlanerName: "Test Planer", GeneratorName: "Prof. Dr. Edda Eich-Söllner"}},
		{"invigilationsSecretariatEmail", "invigilationsSecretariatEmail.md.tmpl", false,
			&secretariatInvigEmail{SemesterName: "2026 SS", PlanerName: "Test Planer"}},
		{"invigilationMissingEmail", "invigilationMissingEmail.md.tmpl", true,
			&InvigilationMissingMailData{Teacher: teacher, Semester: "2026 SS", PlanerName: "Test Planer", Minutes: 180}},
	}
	for _, c := range cases {
		text, html, err := (&Plexams{}).renderMarkdownEmail(c.tmpl, c.jira, c.data)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		assertGolden(t, c.name+".txt", text)
		assertGolden(t, c.name+".html", html)
	}
}

// TestBatch3EmailsGolden locks the invigilation-request and unplanned-exam mails.
func TestBatch3EmailsGolden(t *testing.T) {
	text, html, err := (&Plexams{}).renderMarkdownEmail("invigilationEmail.md.tmpl", true,
		&ConstraintsEmail{FeedbackDate: "13.07.26", PlanerName: "Test Planer"})
	if err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "invigilationEmail.txt", text)
	assertGolden(t, "invigilationEmail.html", html)

	textU, htmlU, err := (&Plexams{}).renderMarkdownEmail("unplannedExamEmail.md.tmpl", false,
		&UnpplannedExamMailData{Exam: &model.PrimussExam{MainExamer: "Prof. Test", AnCode: 123, Module: "Mathe", Program: "IF"}, PlanerName: "Test Planer"})
	if err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "unplannedExamEmail.txt", textU)
	assertGolden(t, "unplannedExamEmail.html", htmlU)
}

// TestBatch3bEmailsGolden locks the four nested-list emails (rooms, KDP, LBA repeaters).
func TestBatch3bEmailsGolden(t *testing.T) {
	rooms := []*roomRequestEmailRoom{{
		Room: "R1.234",
		Days: []*roomRequestEmailDay{{
			Date:  "Mo, 06.07.2026",
			Times: []*roomRequestEmailTime{{From: "08:15", Until: "10:15"}, {From: "10:15", Until: "12:15"}},
		}},
	}}
	kdp := &KdpEmail{SemesterName: "2026 SS", PlanerName: "Test Planer", Slots: []*kdpSlot{{
		Date: "Mo, 06.07.2026", Time: "08:30",
		Rooms: []*kdpRoom{{RoomName: "T3.023", Exams: []*kdpExamInRoom{
			{Ancode: 111, Module: "Mathe", Examer: "Prof. A", Type: "EXaHM", Seats: 30, Detail: "30 Plätze, 90 Min."},
			{Ancode: 222, Module: "Physik", Examer: "Prof. B", Type: "SEB", Seats: 5, Detail: "5 Plätze, 90 Min."},
		}}},
	}}}
	lba := &LbaRepeaterEmail{SemesterName: "2026 SS", PlanerName: "Test Planer", Exams: []*lbaRepeaterExam{{
		Module: "Programmieren", Examer: lbaPerson{Name: "LBA X", Email: "x@hm.edu"},
		Date: "Mo, 06.07.2026", Time: "08:30",
		Programs:     []lbaProgram{{Name: "IF", Count: 3}, {Name: "IB", Count: 1}},
		Invigilators: []lbaPerson{{Name: "Prof. C", Email: "c@hm.edu"}},
	}}}

	cases := []struct {
		name string
		tmpl string
		data any
	}{
		{"roomRequestEmail", "roomRequestEmail.md.tmpl", &RoomRequestEmail{SemesterName: "2026 SS", PlanerName: "Test Planer", Rooms: rooms}},
		{"roomsSecretariatEmail", "roomsSecretariatEmail.md.tmpl", &SecretariatRoomsEmail{SemesterName: "2026 SS", PlanerName: "Test Planer", Rooms: rooms}},
		{"kdpExahmEmail", "kdpExahmEmail.md.tmpl", kdp},
		{"lbaRepeaterEmail", "lbaRepeaterEmail.md.tmpl", lba},
	}
	for _, c := range cases {
		text, html, err := (&Plexams{}).renderMarkdownEmail(c.tmpl, false, c.data)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		assertGolden(t, c.name+".txt", text)
		assertGolden(t, c.name+".html", html)
	}
}
