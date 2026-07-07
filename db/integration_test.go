package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/mongotest"
)

// TestPlanEntryStarttimeStorage verifies that Starttime is the persisted source of
// truth and that a start-time query resolves the entry.
func TestPlanEntryStarttimeStorage(t *testing.T) {
	d := mongotest.NewDB(t)
	ctx := context.Background()

	st := time.Date(2026, 7, 6, 8, 30, 0, 0, time.Local)
	if _, err := d.AddExamToSlot(ctx, &model.PlanEntry{Ancode: 7, Starttime: &st}); err != nil {
		t.Fatal(err)
	}

	pe, err := d.PlanEntry(ctx, 7)
	if err != nil || pe == nil {
		t.Fatalf("PlanEntry: %v (pe=%v)", err, pe)
	}
	if pe.Starttime == nil || !pe.Starttime.Equal(st) {
		t.Errorf("Starttime not round-tripped, got %v", pe.Starttime)
	}

	at, err := d.PlanEntriesAt(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	if len(at) != 1 || at[0].Ancode != 7 {
		t.Errorf("PlanEntriesAt(st) = %+v, want ancode 7", at)
	}
}

// TestRoomStorageStarttime verifies that planned rooms and blocked rooms persist
// Starttime as the source of truth and that the start-time queries resolve them.
func TestRoomStorageStarttime(t *testing.T) {
	d := mongotest.NewDB(t)
	ctx := context.Background()

	st := time.Date(2026, 7, 6, 8, 30, 0, 0, time.Local)
	if err := d.ReplacePlannedRooms(ctx, []*model.PlannedRoom{
		{Starttime: &st, RoomName: "R1.234", Ancode: 7, Duration: 90},
	}); err != nil {
		t.Fatal(err)
	}

	at, err := d.PlannedRoomsAt(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	if len(at) != 1 {
		t.Fatalf("PlannedRoomsAt(st) = %d rooms, want 1", len(at))
	}
	if at[0].Starttime == nil || !at[0].Starttime.Equal(st) {
		t.Errorf("Starttime not round-tripped, got %v", at[0].Starttime)
	}

	names, err := d.PlannedRoomNamesAt(ctx, st)
	if err != nil || len(names) != 1 || names[0] != "R1.234" {
		t.Errorf("PlannedRoomNamesAt(st) = %v (err %v), want [R1.234]", names, err)
	}

	// blocked room: keyed by room + starttime
	if err := d.BlockRoomForSlot(ctx, &model.BlockedRoom{Starttime: &st, Room: "R1.234"}); err != nil {
		t.Fatal(err)
	}
	blocked, err := d.BlockedRooms(ctx)
	if err != nil || len(blocked) != 1 {
		t.Fatalf("BlockedRooms = %d (err %v), want 1", len(blocked), err)
	}
	if blocked[0].Starttime == nil || !blocked[0].Starttime.Equal(st) {
		t.Errorf("blocked starttime not round-tripped, got %v", blocked[0].Starttime)
	}
	removed, err := d.UnblockRoomForSlot(ctx, "R1.234", st)
	if err != nil || !removed {
		t.Errorf("UnblockRoomForSlot = %v (err %v), want true", removed, err)
	}
}

// TestInvigilationStarttimeStorage verifies that pre-planned invigilations persist
// Starttime as the source of truth and that the start-time lookup resolves them.
func TestInvigilationStarttimeStorage(t *testing.T) {
	d := mongotest.NewDB(t)
	ctx := context.Background()

	st := time.Date(2026, 7, 6, 8, 30, 0, 0, time.Local)
	if _, err := d.AddPrePlannedInvigilation(ctx, &model.PrePlannedInvigilation{
		InvigilatorID: 42, Starttime: &st, RoomName: ptr("R1.234"),
	}); err != nil {
		t.Fatal(err)
	}
	ppis, err := d.PrePlannedInvigilations(ctx)
	if err != nil || len(ppis) != 1 {
		t.Fatalf("PrePlannedInvigilations = %d (err %v), want 1", len(ppis), err)
	}
	if ppis[0].Starttime == nil || !ppis[0].Starttime.Equal(st) {
		t.Error("pre-planned invigilation lost its starttime")
	}
	removed, err := d.RemovePrePlannedInvigilationAt(ctx, st, ptr("R1.234"))
	if err != nil || !removed {
		t.Errorf("RemovePrePlannedInvigilationAt = %v (err %v), want true", removed, err)
	}
}

func ptr(s string) *string { return &s }

// TestStarttimeDecodedInLocalZone guards the MongoDB decode-timezone setting
// (SetBSONOptions UseLocalTimeZone: true in NewDB). BSON stores datetimes as
// UTC; without that option the driver would hand them back in UTC, and the
// GraphQL layer would then serve them with a trailing "Z" — breaking the
// frontend's slot derivation and plannedStarttime===slot.starttime matching.
//
// The existing decoration tests compare with time.Time.Equal, which is
// instant-based and passes regardless of zone, so they do NOT catch this. Here
// we pin time.Local to Berlin (as main.go does in production) and assert the
// round-tripped value serializes with a Berlin offset, not Z.
func TestStarttimeDecodedInLocalZone(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatalf("load Europe/Berlin: %v", err)
	}
	orig := time.Local
	time.Local = berlin
	defer func() { time.Local = orig }()

	d := mongotest.NewDB(t)
	ctx := context.Background()

	st := time.Date(2026, 2, 2, 8, 30, 0, 0, berlin) // winter -> +01:00
	if _, err := d.AddExamToSlot(ctx, &model.PlanEntry{Ancode: 7, Starttime: &st}); err != nil {
		t.Fatal(err)
	}
	pe, err := d.PlanEntry(ctx, 7)
	if err != nil || pe == nil || pe.Starttime == nil {
		t.Fatalf("PlanEntry: %v (pe=%v)", err, pe)
	}

	want := marshalGQLTime(st)
	got := marshalGQLTime(*pe.Starttime)
	if got != want {
		t.Fatalf("round-tripped starttime serializes as %q, want %q; "+
			"MongoDB UseLocalTimeZone decode option likely dropped", got, want)
	}
}

// TestResetGeneratedPlanEntries locks the reset semantics: only generated placements are
// removed; manual locks, external / not-planned-by-me and phase-fixed entries survive.
func TestResetGeneratedPlanEntries(t *testing.T) {
	d := mongotest.NewDB(t)
	ctx := context.Background()

	ext := time.Date(2026, 7, 6, 11, 0, 0, 0, time.Local)
	add := func(pe *model.PlanEntry) {
		if _, err := d.AddExamToSlot(ctx, pe); err != nil {
			t.Fatalf("add %d: %v", pe.Ancode, err)
		}
	}
	st := time.Date(2026, 7, 6, 8, 30, 0, 0, time.Local)
	add(&model.PlanEntry{Ancode: 100, Starttime: &st})                   // generated
	add(&model.PlanEntry{Ancode: 200, Starttime: &st, Locked: true})     // manual lock
	add(&model.PlanEntry{Ancode: 300, Starttime: &st, PhaseFixed: true}) // EXaHM/SEB freeze
	add(&model.PlanEntry{Ancode: 400, Starttime: &ext, External: true})  // external

	n, err := d.ResetGeneratedPlanEntries(ctx)
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	if n != 1 {
		t.Errorf("removed %d, want 1", n)
	}

	remaining, err := d.PlanEntries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := map[int]bool{}
	for _, pe := range remaining {
		got[pe.Ancode] = true
	}
	if got[100] {
		t.Errorf("generated entry 100 should be gone")
	}
	for _, a := range []int{200, 300, 400} {
		if !got[a] {
			t.Errorf("entry %d should survive reset", a)
		}
	}
}

// TestPlanEntryLockAndPhaseFixed round-trips the lock and phase-fixed flags.
func TestPlanEntryLockAndPhaseFixed(t *testing.T) {
	d := mongotest.NewDB(t)
	ctx := context.Background()

	st42 := time.Date(2026, 7, 6, 12, 30, 0, 0, time.Local)
	if _, err := d.AddExamToSlot(ctx, &model.PlanEntry{Ancode: 42, Starttime: &st42}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.LockExam(ctx, 42); err != nil {
		t.Fatal(err)
	}
	if err := d.SetPhaseFixed(ctx, 42, true); err != nil {
		t.Fatal(err)
	}
	pe, err := d.PlanEntry(ctx, 42)
	if err != nil {
		t.Fatal(err)
	}
	if pe == nil || !pe.Locked || !pe.PhaseFixed {
		t.Fatalf("want locked+phaseFixed, got %+v", pe)
	}

	if err := d.ClearAllPhaseFixed(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := d.UnlockExam(ctx, 42); err != nil {
		t.Fatal(err)
	}
	pe, _ = d.PlanEntry(ctx, 42)
	if pe.Locked || pe.PhaseFixed {
		t.Errorf("flags should be cleared, got %+v", pe)
	}
}

// TestNotPlannedByMeWithFaculty locks the not-planned-by-me flag plus its faculty.
func TestNotPlannedByMeWithFaculty(t *testing.T) {
	d := mongotest.NewDB(t)
	ctx := context.Background()

	fk := "FK10"
	if _, err := d.NotPlannedByMe(ctx, 555, &fk); err != nil {
		t.Fatal(err)
	}
	c, err := d.GetConstraintsForAncode(ctx, 555)
	if err != nil {
		t.Fatal(err)
	}
	if c == nil || !c.NotPlannedByMe {
		t.Fatalf("expected notPlannedByMe, got %+v", c)
	}
	if c.NotPlannedByMeInFk == nil || *c.NotPlannedByMeInFk != "FK10" {
		t.Errorf("expected inFK FK10, got %v", c.NotPlannedByMeInFk)
	}
}

// TestSemesterConfigExamGapRoundTrip locks the GUI-configurable exam-gap value.
func TestSemesterConfigExamGapRoundTrip(t *testing.T) {
	d := mongotest.NewDB(t)
	ctx := context.Background()

	gap := 45
	lag := 25
	tooClose := 90
	in := &model.SemesterConfigInput{
		From:               time.Date(2026, 7, 6, 0, 0, 0, 0, time.Local),
		Until:              time.Date(2026, 7, 17, 0, 0, 0, 0, time.Local),
		StartTimes:         []string{"08:30", "10:30"},
		Emails:             &model.Emails{},
		ExamGapMinutes:     &gap,
		TimelagMin:         &lag,
		NotTooCloseMinutes: &tooClose,
	}
	if err := d.SaveSemesterConfigInput(ctx, in); err != nil {
		t.Fatal(err)
	}
	out, err := d.GetSemesterConfigInput(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if out == nil || out.ExamGapMinutes == nil || *out.ExamGapMinutes != 45 {
		t.Errorf("examGapMinutes not round-tripped, got %+v", out)
	}
	if out == nil || out.TimelagMin == nil || *out.TimelagMin != 25 {
		t.Errorf("timelagMin not round-tripped, got %+v", out)
	}
	if out == nil || out.NotTooCloseMinutes == nil || *out.NotTooCloseMinutes != 90 {
		t.Errorf("notTooCloseMinutes not round-tripped, got %+v", out)
	}
}

// TestExternalExamFaculty locks the external exam's stamped origin faculty.
func TestExternalExamFaculty(t *testing.T) {
	d := mongotest.NewDB(t)
	ctx := context.Background()

	exam := &model.ZPAExam{
		AnCode:         90001,
		Module:         "Mathe",
		Faculty:        "FK03",
		PrimussAncodes: []model.ZPAPrimussAncodes{{Program: "DE", Ancode: 12}},
	}
	if err := d.AddExternalExam(ctx, exam); err != nil {
		t.Fatal(err)
	}
	if err := d.SetExternalExamFaculty(ctx, 90001, "FK08"); err != nil {
		t.Fatal(err)
	}
	exams, err := d.ExternalExams(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(exams) != 1 || exams[0].Faculty != "FK08" {
		t.Errorf("faculty update not persisted, got %+v", exams)
	}
}

// TestConflictRatings locks the per-student decision and can-share-slot storage.
func TestConflictRatings(t *testing.T) {
	d := mongotest.NewDB(t)
	ctx := context.Background()

	if err := d.UpsertDecision(ctx, 100, 200, "mtk1", "ACCEPT"); err != nil {
		t.Fatal(err)
	}
	decs, err := d.StudentConflictDecisions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(decs) != 1 || decs[0].Mtknr != "mtk1" {
		t.Fatalf("decision not stored: %+v", decs)
	}

	if err := d.UpsertCanShareSlot(ctx, 300, 400); err != nil {
		t.Fatal(err)
	}
	pairs, err := d.CanShareSlotPairs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pairs) != 1 || pairs[0] != [2]int{300, 400} {
		t.Errorf("canShareSlot pair not stored: %+v", pairs)
	}
}
