package db_test

import (
	"testing"
	"time"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/internal/pgtest"
)

func TestSchedulerStateMissingReturnsNilNil(t *testing.T) {
	pg := pgtest.NewDB(t)

	got, err := pg.GetSchedulerState(t.Context())
	if err != nil {
		t.Fatalf("GetSchedulerState: %v", err)
	}
	if got != nil {
		t.Errorf("GetSchedulerState = %v, want nil -- a fresh deploy must not trigger a catch-up", got)
	}
}

// TestSchedulerStateFirstFireHasNoOutcome is why last_finished and last_status
// are nullable. TouchSchedulerFire writes the anchor before the run executes, so
// between it and the first completed run there is genuinely no outcome; NOT NULL
// would have forced a fabricated one, and the empty string is not an allowed
// status either.
func TestSchedulerStateFirstFireHasNoOutcome(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	at := time.Date(2026, 8, 4, 3, 0, 0, 0, time.Local)
	if err := pg.TouchSchedulerFire(ctx, at, "nightly", "2026-WS"); err != nil {
		t.Fatalf("TouchSchedulerFire: %v", err)
	}

	got, err := pg.GetSchedulerState(ctx)
	if err != nil {
		t.Fatalf("GetSchedulerState: %v", err)
	}
	if got == nil {
		t.Fatal("scheduler state is nil right after a fire was recorded")
	}
	if !got.LastFireAt.Equal(at) {
		t.Errorf("LastFireAt = %v, want %v", got.LastFireAt, at)
	}
	if got.LastFireAt.Location() != time.Local {
		t.Errorf("LastFireAt location = %v, want time.Local", got.LastFireAt.Location())
	}
	if got.LastTrigger != "nightly" {
		t.Errorf("LastTrigger = %q, want %q", got.LastTrigger, "nightly")
	}
	if got.Semester != "2026-WS" {
		t.Errorf("Semester = %q, want %q", got.Semester, "2026-WS")
	}
	if !got.LastFinished.IsZero() {
		t.Errorf("LastFinished = %v, want the zero time", got.LastFinished)
	}
	if got.LastStatus != "" {
		t.Errorf("LastStatus = %q, want the empty string", got.LastStatus)
	}
}

func TestSchedulerStateRoundTrip(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	want := &db.SchedulerState{
		LastFireAt:   time.Date(2026, 8, 4, 3, 0, 0, 0, time.Local),
		LastFinished: time.Date(2026, 8, 4, 3, 4, 17, 0, time.Local),
		LastStatus:   "ok",
		LastTrigger:  "nightly",
		Semester:     "2026-WS",
		TotalChanges: 7,
	}
	if err := pg.SaveSchedulerState(ctx, want); err != nil {
		t.Fatalf("SaveSchedulerState: %v", err)
	}

	got, err := pg.GetSchedulerState(ctx)
	if err != nil {
		t.Fatalf("GetSchedulerState: %v", err)
	}
	if got == nil {
		t.Fatal("scheduler state is nil")
	}
	if !got.LastFireAt.Equal(want.LastFireAt) {
		t.Errorf("LastFireAt = %v, want %v", got.LastFireAt, want.LastFireAt)
	}
	if !got.LastFinished.Equal(want.LastFinished) {
		t.Errorf("LastFinished = %v, want %v", got.LastFinished, want.LastFinished)
	}
	if got.LastStatus != want.LastStatus {
		t.Errorf("LastStatus = %q, want %q", got.LastStatus, want.LastStatus)
	}
	if got.LastTrigger != want.LastTrigger {
		t.Errorf("LastTrigger = %q, want %q", got.LastTrigger, want.LastTrigger)
	}
	if got.Semester != want.Semester {
		t.Errorf("Semester = %q, want %q", got.Semester, want.Semester)
	}
	if got.TotalChanges != want.TotalChanges {
		t.Errorf("TotalChanges = %d, want %d", got.TotalChanges, want.TotalChanges)
	}
}

// TestSchedulerStateTouchKeepsTheOutcome is the whole reason TouchSchedulerFire
// exists as its own method: it moves the catch-up anchor without erasing what the
// last completed run reported.
func TestSchedulerStateTouchKeepsTheOutcome(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	finished := time.Date(2026, 8, 4, 3, 4, 17, 0, time.Local)
	if err := pg.SaveSchedulerState(ctx, &db.SchedulerState{
		LastFireAt:   time.Date(2026, 8, 4, 3, 0, 0, 0, time.Local),
		LastFinished: finished,
		LastStatus:   "errors",
		LastTrigger:  "nightly",
		Semester:     "2026-WS",
		TotalChanges: 3,
	}); err != nil {
		t.Fatalf("SaveSchedulerState: %v", err)
	}

	nextFire := time.Date(2026, 8, 5, 3, 0, 0, 0, time.Local)
	if err := pg.TouchSchedulerFire(ctx, nextFire, "manual", "Test26SS-v2"); err != nil {
		t.Fatalf("TouchSchedulerFire: %v", err)
	}

	got, err := pg.GetSchedulerState(ctx)
	if err != nil {
		t.Fatalf("GetSchedulerState: %v", err)
	}
	if !got.LastFireAt.Equal(nextFire) {
		t.Errorf("LastFireAt = %v, want %v", got.LastFireAt, nextFire)
	}
	if got.LastTrigger != "manual" {
		t.Errorf("LastTrigger = %q, want %q", got.LastTrigger, "manual")
	}
	if got.Semester != "Test26SS-v2" {
		t.Errorf("Semester = %q, want %q", got.Semester, "Test26SS-v2")
	}
	// ... and the previous run's outcome is untouched.
	if !got.LastFinished.Equal(finished) {
		t.Errorf("LastFinished = %v, want %v -- the touch clobbered the outcome", got.LastFinished, finished)
	}
	if got.LastStatus != "errors" {
		t.Errorf("LastStatus = %q, want %q -- the touch clobbered the outcome", got.LastStatus, "errors")
	}
	if got.TotalChanges != 3 {
		t.Errorf("TotalChanges = %d, want 3 -- the touch clobbered the outcome", got.TotalChanges)
	}
}

func TestSchedulerStateIsASingleton(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	at := time.Date(2026, 8, 4, 3, 0, 0, 0, time.Local)
	if err := pg.TouchSchedulerFire(ctx, at, "nightly", "2026-WS"); err != nil {
		t.Fatalf("TouchSchedulerFire: %v", err)
	}
	if err := pg.TouchSchedulerFire(ctx, at.Add(24*time.Hour), "catchup", "2026-WS"); err != nil {
		t.Fatalf("TouchSchedulerFire (second): %v", err)
	}

	if n := count(t, pg, "select count(*) from scheduler_state"); n != 1 {
		t.Errorf("scheduler_state rows = %d, want 1", n)
	}
}

// An unknown status must not reach the table: the heartbeat mail renders it
// verbatim, and a typo would be indistinguishable from a real outcome.
func TestSchedulerStateUnknownStatusIsRejected(t *testing.T) {
	pg := pgtest.NewDB(t)

	err := pg.SaveSchedulerState(t.Context(), &db.SchedulerState{
		LastFireAt:   time.Now(),
		LastFinished: time.Now(),
		LastStatus:   "kaputt",
		LastTrigger:  "nightly",
	})
	if err == nil {
		t.Error("SaveSchedulerState accepted a status outside the allowed set")
	}
}
