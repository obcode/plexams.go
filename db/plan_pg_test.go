package db_test

import (
	"testing"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
)

// berlin parses a wall-clock time in time.Local, which main.go pins to
// Europe/Berlin. Deliberately not time.LoadLocation("Europe/Berlin"): that
// returns a different *time.Location pointer, and a time built from it is a
// different map key for the same instant. See TestTimestamptzKeepsLocation and
// TestSeparatelyLoadedLocationIsADifferentMapKey.
func berlin(t *testing.T, value string) time.Time {
	t.Helper()
	ts, err := time.ParseInLocation("2006-01-02 15:04", value, time.Local)
	if err != nil {
		t.Fatalf("parse %q: %v", value, err)
	}
	return ts
}

// TestAddExamToSlotReplacesTheEntry pins the semantics of the Mongo version,
// which deleted every entry for the ancode and inserted the new one: the flags
// come from the entry handed in, not from what was stored before.
func TestAddExamToSlotReplacesTheEntry(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg, 100)
	first := berlin(t, "2027-01-20 08:30")
	second := berlin(t, "2027-01-22 14:00")

	if _, err := pg.AddExamToSlot(ctx, &model.PlanEntry{
		Ancode: 100, Starttime: &first, Locked: true, PhaseFixed: true,
	}); err != nil {
		t.Fatalf("AddExamToSlot: %v", err)
	}
	if _, err := pg.AddExamToSlot(ctx, &model.PlanEntry{
		Ancode: 100, Starttime: &second,
	}); err != nil {
		t.Fatalf("AddExamToSlot again: %v", err)
	}

	if n := count(t, pg, `select count(*) from plan_entry where semester_id='2026-WS'`); n != 1 {
		t.Fatalf("plan_entry count = %d, want 1 -- placing an exam twice must not duplicate it", n)
	}

	entry, err := pg.PlanEntry(ctx, 100)
	if err != nil {
		t.Fatalf("PlanEntry: %v", err)
	}
	if !entry.Starttime.Equal(second) {
		t.Errorf("Starttime = %v, want %v", entry.Starttime, second)
	}
	if entry.Locked || entry.PhaseFixed {
		t.Errorf("Locked=%v PhaseFixed=%v, want both false -- the replacement carried neither",
			entry.Locked, entry.PhaseFixed)
	}
}

// The start time must come back in Europe/Berlin. A UTC location is a *different*
// map key for the same instant, and plexams keys several maps by start time.
func TestPlanEntryKeepsTheBerlinLocation(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg, 100)
	start := berlin(t, "2027-01-20 08:30")
	if _, err := pg.AddExamToSlot(ctx, &model.PlanEntry{Ancode: 100, Starttime: &start}); err != nil {
		t.Fatalf("AddExamToSlot: %v", err)
	}

	entries, err := pg.PlanEntriesAt(ctx, start)
	if err != nil {
		t.Fatalf("PlanEntriesAt: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("PlanEntriesAt returned %d entries, want 1", len(entries))
	}
	if got := entries[0].Starttime.Format("02.01.2006 15:04"); got != "20.01.2027 08:30" {
		t.Errorf("wall clock = %q, want 20.01.2027 08:30", got)
	}
	if *entries[0].Starttime != start {
		t.Errorf("Starttime = %#v, want the struct-identical %#v -- a differing location "+
			"is a different map key even though Equal() says true", *entries[0].Starttime, start)
	}
}

// PlanEntry answers (nil, nil) for an exam that is not in the plan. LockExam and
// ExamIsLocked both read that as "not planned"; an error would turn a normal
// answer into a failure.
func TestPlanEntryOfAnUnplannedExamIsNil(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg, 100)

	entry, err := pg.PlanEntry(ctx, 100)
	if err != nil {
		t.Fatalf("PlanEntry: %v", err)
	}
	if entry != nil {
		t.Errorf("PlanEntry = %#v, want nil", entry)
	}
	if pg.ExamIsLocked(ctx, 100) {
		t.Error("ExamIsLocked = true for an exam that is not in the plan")
	}
}

// TestResetKeepsWhatWasNotGenerated is the contract of the plan reset: locked,
// phase-fixed and external entries survive it.
func TestResetKeepsWhatWasNotGenerated(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg, 1, 2, 3, 4)
	start := berlin(t, "2027-01-20 08:30")

	entries := []*model.PlanEntry{
		{Ancode: 1, Starttime: &start},
		{Ancode: 2, Starttime: &start, Locked: true},
		{Ancode: 3, Starttime: &start, PhaseFixed: true},
		{Ancode: 4, Starttime: &start, External: true},
	}
	for _, entry := range entries {
		if _, err := pg.AddExamToSlot(ctx, entry); err != nil {
			t.Fatalf("AddExamToSlot(%d): %v", entry.Ancode, err)
		}
	}

	n, err := pg.ResetGeneratedPlanEntries(ctx)
	if err != nil {
		t.Fatalf("ResetGeneratedPlanEntries: %v", err)
	}
	if n != 1 {
		t.Errorf("removed %d entries, want 1", n)
	}

	left, err := pg.PlanEntries(ctx)
	if err != nil {
		t.Fatalf("PlanEntries: %v", err)
	}
	if len(left) != 3 {
		t.Fatalf("%d entries left, want 3", len(left))
	}
	for _, entry := range left {
		if entry.Ancode == 1 {
			t.Error("the generated entry survived the reset")
		}
	}
}

// Resetting the plan takes the rooms of the reset exams with it. Under MongoDB
// they stayed behind pointing at an exam that was no longer planned -- one of the
// two things plexams/validate_db.go:253 had to look for.
func TestResettingThePlanTakesTheRoomsWithIt(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg, 1, 2)
	exec(t, pg, `insert into room (name, seats) values ('R1.046', 60)`)
	start := berlin(t, "2027-01-20 08:30")

	if _, err := pg.AddExamToSlot(ctx, &model.PlanEntry{Ancode: 1, Starttime: &start}); err != nil {
		t.Fatalf("AddExamToSlot(1): %v", err)
	}
	if _, err := pg.AddExamToSlot(ctx, &model.PlanEntry{Ancode: 2, Starttime: &start, Locked: true}); err != nil {
		t.Fatalf("AddExamToSlot(2): %v", err)
	}
	if err := pg.ReplacePlannedRooms(ctx, []*model.PlannedRoom{
		{Ancode: 1, RoomName: "R1.046", Duration: 90},
		{Ancode: 2, RoomName: "R1.046", Duration: 90},
	}); err != nil {
		t.Fatalf("ReplacePlannedRooms: %v", err)
	}

	if _, err := pg.ResetGeneratedPlanEntries(ctx); err != nil {
		t.Fatalf("ResetGeneratedPlanEntries: %v", err)
	}

	rooms, err := pg.PlannedRooms(ctx)
	if err != nil {
		t.Fatalf("PlannedRooms: %v", err)
	}
	if len(rooms) != 1 || rooms[0].Ancode != 2 {
		t.Errorf("planned rooms after reset = %+v, want only the locked exam's room", rooms)
	}
}

func TestLockAndUnlockExam(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg, 100)
	start := berlin(t, "2027-01-20 08:30")
	if _, err := pg.AddExamToSlot(ctx, &model.PlanEntry{Ancode: 100, Starttime: &start}); err != nil {
		t.Fatalf("AddExamToSlot: %v", err)
	}

	entry, err := pg.LockExam(ctx, 100)
	if err != nil {
		t.Fatalf("LockExam: %v", err)
	}
	if !entry.Locked {
		t.Error("LockExam returned an unlocked entry")
	}
	if !pg.ExamIsLocked(ctx, 100) {
		t.Error("ExamIsLocked = false after LockExam")
	}

	entry, err = pg.UnlockExam(ctx, 100)
	if err != nil {
		t.Fatalf("UnlockExam: %v", err)
	}
	if entry.Locked {
		t.Error("UnlockExam returned a locked entry")
	}

	// The start time must survive both -- Mongo used $set on the one field.
	if !entry.Starttime.Equal(start) {
		t.Errorf("Starttime = %v after lock/unlock, want %v", entry.Starttime, start)
	}
}

// Locking an exam that is not in the plan is not an error: the Mongo version
// updated nothing and returned the (nil) entry it then read back.
func TestLockingAnUnplannedExamIsNotAnError(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg, 100)

	entry, err := pg.LockExam(ctx, 100)
	if err != nil {
		t.Fatalf("LockExam: %v", err)
	}
	if entry != nil {
		t.Errorf("LockExam = %#v, want nil", entry)
	}
}

func TestLockPlanAndPhaseFixed(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg, 1, 2)
	start := berlin(t, "2027-01-20 08:30")
	for _, ancode := range []int{1, 2} {
		if _, err := pg.AddExamToSlot(ctx, &model.PlanEntry{Ancode: ancode, Starttime: &start}); err != nil {
			t.Fatalf("AddExamToSlot(%d): %v", ancode, err)
		}
	}

	if err := pg.SetPhaseFixed(ctx, 1, true); err != nil {
		t.Fatalf("SetPhaseFixed: %v", err)
	}
	if err := pg.LockPlan(ctx); err != nil {
		t.Fatalf("LockPlan: %v", err)
	}

	entries, err := pg.PlanEntries(ctx)
	if err != nil {
		t.Fatalf("PlanEntries: %v", err)
	}
	for _, entry := range entries {
		if !entry.Locked {
			t.Errorf("ancode %d not locked after LockPlan", entry.Ancode)
		}
	}

	if err := pg.ClearAllPhaseFixed(ctx); err != nil {
		t.Fatalf("ClearAllPhaseFixed: %v", err)
	}
	entries, err = pg.PlanEntries(ctx)
	if err != nil {
		t.Fatalf("PlanEntries: %v", err)
	}
	for _, entry := range entries {
		if entry.PhaseFixed {
			t.Errorf("ancode %d still phase-fixed", entry.Ancode)
		}
		// ClearAllPhaseFixed must not undo the lock -- Mongo set the one field.
		if !entry.Locked {
			t.Errorf("ancode %d lost its lock to ClearAllPhaseFixed", entry.Ancode)
		}
	}
}

// An exam can be in the plan without a time: model.PlanEntry.IsPlanned() is
// exactly this distinction, so the column has to stay nullable.
func TestAPlanEntryWithoutATimeIsNotPlanned(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg, 100)
	if _, err := pg.AddExamToSlot(ctx, &model.PlanEntry{Ancode: 100}); err != nil {
		t.Fatalf("AddExamToSlot: %v", err)
	}

	entry, err := pg.PlanEntry(ctx, 100)
	if err != nil {
		t.Fatalf("PlanEntry: %v", err)
	}
	if entry == nil || entry.IsPlanned() {
		t.Errorf("entry = %#v, want a stored entry that is not planned", entry)
	}
}
