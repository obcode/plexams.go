package plexams

import (
	"context"
	"testing"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
	"github.com/obcode/plexams.go/plexams/apierr"
)

// A lock is the planner's "keep this exam here": moving the exam by hand keeps it,
// and an exam without a time cannot be locked.
func TestExamLockSurvivesAManualMove(t *testing.T) {
	ctx := context.Background()
	pg := pgtest.NewDBWithSemester(t)
	p := &Plexams{dbClient: pg}
	if err := pg.CacheZPAExams([]*model.ZPAExam{{
		AnCode: 100, Module: "Analysis", MainExamer: "Test", MainExamerID: 1,
		ExamType: "sp", ExamTypeFull: "schriftliche Prüfung", Groups: []string{},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := pg.CacheAssembledExams(ctx, []*model.AssembledExam{{Ancode: 100, MaxDuration: 90}}); err != nil {
		t.Fatal(err)
	}

	if _, err := p.SetExamLocked(ctx, 100, true); apierr.Code(err) != apierr.CodeNotFound {
		t.Fatalf("locking an exam without a time: err = %v, want NOT_FOUND", err)
	}

	first := time.Date(2027, 1, 25, 10, 30, 0, 0, time.Local)
	if _, err := p.SetExamTime(ctx, 100, first); err != nil {
		t.Fatal(err)
	}
	if entry, err := p.SetExamLocked(ctx, 100, true); err != nil || entry == nil || !entry.Locked {
		t.Fatalf("SetExamLocked = %+v, %v; want a locked entry", entry, err)
	}

	second := first.Add(2 * time.Hour)
	if _, err := p.SetExamTime(ctx, 100, second); err != nil {
		t.Fatal(err)
	}
	entry, err := pg.PlanEntry(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !entry.Locked || !entry.Starttime.Equal(second) {
		t.Errorf("after the move: locked=%v at %v, want locked at %v", entry.Locked, entry.Starttime, second)
	}

	if entry, err := p.SetExamLocked(ctx, 100, false); err != nil || entry.Locked {
		t.Fatalf("unlock = %+v, %v; want an unlocked entry", entry, err)
	}
}
