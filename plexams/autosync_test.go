package plexams

import (
	"strings"
	"testing"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/email"
)

func report(sources ...*syncSourceResult) *SyncRunReport {
	r := &SyncRunReport{
		Semester: "2026 WS",
		Started:  time.Date(2026, 7, 15, 3, 0, 0, 0, time.Local),
		Finished: time.Date(2026, 7, 15, 3, 2, 0, 0, time.Local),
		Sources:  sources,
	}
	for _, s := range sources {
		r.TotalChanges += s.changes()
	}
	return r
}

// renderReport builds the subject and renders the report template (embedded default, no
// store) exactly as the nightly mail does, so the tests exercise the real Markdown output.
func renderReport(t *testing.T, r *SyncRunReport) (subject, text, html string) {
	t.Helper()
	subject = buildSyncReportSubject(r)
	txt, h, err := email.New(nil, renderFuncs(), jiraURL).Render("autoSyncReport.md.tmpl", false, newSyncReportView(r))
	if err != nil {
		t.Fatalf("render autoSyncReport.md.tmpl: %v", err)
	}
	return subject, string(txt), string(h)
}

func TestSyncReportNoChanges(t *testing.T) {
	subject, text, html := renderReport(t, report(&syncSourceResult{
		key: "zpa-import-exams", label: "Prüfungen (ZPA)", noun: "Prüfungen", count: 120,
		entry: &model.SyncLogEntry{},
	}))
	if !strings.Contains(subject, "keine Änderungen") {
		t.Errorf("subject = %q, want 'keine Änderungen'", subject)
	}
	if !strings.Contains(text, "Keine Änderungen (120 Prüfungen).") {
		t.Errorf("body missing unchanged-with-data line:\n%s", text)
	}
	if len(html) == 0 {
		t.Error("rendered HTML is empty")
	}
}

func TestSyncReportEmptySource(t *testing.T) {
	// ZPA has no exams for a fresh semester yet: count 0, no error → not a failure.
	subject, text, _ := renderReport(t, report(&syncSourceResult{
		key: "zpa-import-exams", label: "Prüfungen (ZPA)", noun: "Prüfungen", count: 0,
		entry: &model.SyncLogEntry{},
	}))
	if !strings.Contains(subject, "keine Änderungen") {
		t.Errorf("subject = %q, want 'keine Änderungen' (empty source is not an error)", subject)
	}
	if !strings.Contains(text, "Keine Prüfungen vorhanden.") {
		t.Errorf("body missing empty-source line:\n%s", text)
	}
}

func TestSyncReportWithChanges(t *testing.T) {
	entry := &model.SyncLogEntry{
		Added: 1, Changed: 1,
		Entries: []*model.SyncChangeEntry{
			{Type: "added", Name: "999. Neu (Prof)"},
			{Type: "changed", Name: "45. Alt (Prof)", Fields: []*model.SyncFieldChange{
				{Field: "duration", Old: "90", New: "120"},
			}},
		},
	}
	subject, text, _ := renderReport(t, report(&syncSourceResult{
		key: "zpa-import-exams", label: "Prüfungen (ZPA)", entry: entry,
	}))
	if !strings.Contains(subject, "2 Änderungen") {
		t.Errorf("subject = %q, want '2 Änderungen'", subject)
	}
	if !strings.Contains(text, "neu:") || !strings.Contains(text, "999. Neu (Prof)") {
		t.Errorf("body missing added line:\n%s", text)
	}
	if !strings.Contains(text, `duration: "90" → "120"`) {
		t.Errorf("body missing field change:\n%s", text)
	}
}

func TestSyncReportCompaction(t *testing.T) {
	entries := make([]*model.SyncChangeEntry, 0, maxDetailLines+5)
	for i := 0; i < maxDetailLines+5; i++ {
		entries = append(entries, &model.SyncChangeEntry{Type: "added", Name: "exam"})
	}
	entry := &model.SyncLogEntry{Added: len(entries), Entries: entries}
	_, text, _ := renderReport(t, report(&syncSourceResult{
		key: "zpa-import-exams", label: "Prüfungen (ZPA)", entry: entry,
	}))
	if !strings.Contains(text, "Detailliste ausgelassen") {
		t.Errorf("expected compaction note for large change set:\n%s", text)
	}
	if strings.Contains(text, "neu:** exam") {
		t.Errorf("large change set should not list individual entries:\n%s", text)
	}
}

func TestSyncReportSkipped(t *testing.T) {
	r := &SyncRunReport{
		Semester:   "2026 WS",
		Started:    time.Date(2026, 7, 15, 3, 0, 0, 0, time.Local),
		Finished:   time.Date(2026, 7, 15, 3, 0, 0, 0, time.Local),
		Skipped:    true,
		SkipReason: "andere Operation lief",
	}
	subject, text, _ := renderReport(t, r)
	if !strings.Contains(subject, "übersprungen") {
		t.Errorf("subject = %q, want 'übersprungen'", subject)
	}
	if !strings.Contains(text, "übersprungen") || !strings.Contains(text, "andere Operation lief") {
		t.Errorf("body missing skip reason:\n%s", text)
	}
}

func TestSyncReportError(t *testing.T) {
	subject, text, _ := renderReport(t, report(&syncSourceResult{
		key: "anny-import-bookings", label: "Anny-Buchungen", err: errFake,
	}))
	if !strings.Contains(subject, "Fehler") {
		t.Errorf("subject = %q, want a 'Fehler' subject", subject)
	}
	if !strings.Contains(text, "Fehler beim Abruf") || !strings.Contains(text, "boom") {
		t.Errorf("body missing error detail:\n%s", text)
	}
}

type fakeErr struct{}

func (fakeErr) Error() string { return "boom" }

var errFake = fakeErr{}
