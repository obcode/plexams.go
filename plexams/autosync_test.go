package plexams

import (
	"strings"
	"testing"
	"time"

	"github.com/obcode/plexams.go/graph/model"
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

func TestBuildSyncReportMailNoChanges(t *testing.T) {
	subject, body := buildSyncReportMail(report(&syncSourceResult{
		key: "zpa-import-exams", label: "Prüfungen (ZPA)", noun: "Prüfungen", count: 120,
		entry: &model.SyncLogEntry{},
	}))
	if !strings.Contains(subject, "keine Änderungen") {
		t.Errorf("subject = %q, want 'keine Änderungen'", subject)
	}
	if !strings.Contains(string(body), "keine Änderungen (120 Prüfungen)") {
		t.Errorf("body missing unchanged-with-data line:\n%s", body)
	}
}

func TestBuildSyncReportMailEmptySource(t *testing.T) {
	// ZPA has no exams for a fresh semester yet: count 0, no error → not a failure.
	subject, body := buildSyncReportMail(report(&syncSourceResult{
		key: "zpa-import-exams", label: "Prüfungen (ZPA)", noun: "Prüfungen", count: 0,
		entry: &model.SyncLogEntry{},
	}))
	if !strings.Contains(subject, "keine Änderungen") {
		t.Errorf("subject = %q, want 'keine Änderungen' (empty source is not an error)", subject)
	}
	if !strings.Contains(string(body), "Prüfungen (ZPA): keine Prüfungen vorhanden") {
		t.Errorf("body missing empty-source line:\n%s", body)
	}
}

func TestBuildSyncReportMailWithChanges(t *testing.T) {
	entry := &model.SyncLogEntry{
		Added: 1, Changed: 1,
		Entries: []*model.SyncChangeEntry{
			{Type: "added", Name: "999. Neu (Prof)"},
			{Type: "changed", Name: "45. Alt (Prof)", Fields: []*model.SyncFieldChange{
				{Field: "duration", Old: "90", New: "120"},
			}},
		},
	}
	subject, body := buildSyncReportMail(report(&syncSourceResult{
		key: "zpa-import-exams", label: "Prüfungen (ZPA)", entry: entry,
	}))
	if !strings.Contains(subject, "2 Änderungen") {
		t.Errorf("subject = %q, want '2 Änderungen'", subject)
	}
	s := string(body)
	if !strings.Contains(s, "+ neu: 999. Neu (Prof)") {
		t.Errorf("body missing added line:\n%s", s)
	}
	if !strings.Contains(s, `duration: "90" → "120"`) {
		t.Errorf("body missing field change:\n%s", s)
	}
}

func TestBuildSyncReportMailCompaction(t *testing.T) {
	entries := make([]*model.SyncChangeEntry, 0, maxDetailLines+5)
	for i := 0; i < maxDetailLines+5; i++ {
		entries = append(entries, &model.SyncChangeEntry{Type: "added", Name: "exam"})
	}
	entry := &model.SyncLogEntry{Added: len(entries), Entries: entries}
	_, body := buildSyncReportMail(report(&syncSourceResult{
		key: "zpa-import-exams", label: "Prüfungen (ZPA)", entry: entry,
	}))
	s := string(body)
	if !strings.Contains(s, "Detailliste ausgelassen") {
		t.Errorf("expected compaction note for large change set:\n%s", s)
	}
	if strings.Contains(s, "+ neu: exam") {
		t.Errorf("large change set should not list individual entries:\n%s", s)
	}
}

func TestBuildSyncReportMailSkipped(t *testing.T) {
	r := &SyncRunReport{Semester: "2026 WS", Skipped: true, SkipReason: "andere Operation lief"}
	subject, body := buildSyncReportMail(r)
	if !strings.Contains(subject, "übersprungen") {
		t.Errorf("subject = %q, want 'übersprungen'", subject)
	}
	if !strings.Contains(string(body), "andere Operation lief") {
		t.Errorf("body missing skip reason:\n%s", body)
	}
}

func TestBuildSyncReportMailError(t *testing.T) {
	subject, _ := buildSyncReportMail(report(&syncSourceResult{
		key: "anny-import-bookings", label: "Anny-Buchungen", err: errFake,
	}))
	if !strings.Contains(subject, "Fehler") {
		t.Errorf("subject = %q, want a 'Fehler' subject", subject)
	}
}

type fakeErr struct{}

func (fakeErr) Error() string { return "boom" }

var errFake = fakeErr{}
