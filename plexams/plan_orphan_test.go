package plexams

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
)

// seedOrphanPlanEntry plans ZPA exams 100 and 101 at start, but only 100 is in the
// assembled-exam cache: 101 is the orphan entry.
func seedOrphanPlanEntry(t *testing.T, start time.Time) *Plexams {
	t.Helper()
	ctx := t.Context()
	pg := pgtest.NewDBWithSemester(t)
	p := &Plexams{
		dbClient:       pg,
		semesterConfig: &model.SemesterConfig{Slots: []*model.Slot{{Starttime: start}}},
	}

	exams := make([]*model.ZPAExam, 0, 2)
	for _, ancode := range []int{100, 101} {
		exams = append(exams, &model.ZPAExam{
			AnCode: ancode, Module: "Modul", MainExamer: "Test", MainExamerID: 1,
			ExamType: "sp", ExamTypeFull: "schriftliche Prüfung", Groups: []string{},
		})
	}
	if err := pg.CacheZPAExams(exams); err != nil {
		t.Fatal(err)
	}
	if err := pg.SetZPAExamsToPlan(ctx, exams, nil); err != nil {
		t.Fatal(err)
	}
	for _, ancode := range []int{100, 101} {
		if _, err := pg.AddExamToSlot(ctx, &model.PlanEntry{Ancode: ancode, Starttime: &start}); err != nil {
			t.Fatal(err)
		}
	}
	if err := pg.CacheAssembledExams(ctx, []*model.AssembledExam{{
		Ancode:  100,
		ZpaExam: exams[0],
	}}); err != nil {
		t.Fatal(err)
	}
	return p
}

// examsAt (used by the self-invigilations and the invigilator ICS) used to fail the
// whole slot on one orphan plan entry. MakeSelfInvigilations then logged an error
// and silently dropped every self-invigilation of that slot.
func TestExamsAtSkipsAnOrphanPlanEntry(t *testing.T) {
	start := time.Date(2027, 2, 3, 10, 30, 0, 0, time.Local)
	p := seedOrphanPlanEntry(t, start)

	exams, err := p.examsAt(context.Background(), start)
	if err != nil {
		t.Fatalf("examsAt: %v -- one orphan entry must not take down the slot", err)
	}
	if len(exams) != 1 || exams[0].Ancode != 100 {
		t.Fatalf("examsAt = %d exams, want only ancode 100", len(exams))
	}
}

// The skipped entry must not disappear silently: the plan-entries validator tells
// the planner about it.
func TestValidateDBPlanEntriesReportsAPlanEntryWithoutAssembledExam(t *testing.T) {
	start := time.Date(2027, 2, 3, 10, 30, 0, 0, time.Local)
	p := seedOrphanPlanEntry(t, start)

	report, err := p.ValidateDBPlanEntries(newDiscardReporter())
	if err != nil {
		t.Fatalf("ValidateDBPlanEntries: %v", err)
	}
	var found []*model.ValidationFinding
	for _, f := range report.Findings {
		if f.Ancode != nil && *f.Ancode == 101 && strings.Contains(f.Message, "no assembled exam") {
			found = append(found, f)
		}
		if f.Ancode != nil && *f.Ancode == 100 {
			t.Errorf("unexpected finding for the assembled exam 100: %s", f.Message)
		}
	}
	if len(found) != 1 {
		t.Fatalf("findings for ancode 101 = %d, want 1 (all: %+v)", len(found), report.Findings)
	}
	if found[0].Level != model.ValidationLevelWarning {
		t.Errorf("level = %s, want WARNING", found[0].Level)
	}
}
