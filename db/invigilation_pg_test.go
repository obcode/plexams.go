package db_test

import (
	"testing"
	"time"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
	"github.com/obcode/plexams.go/zpa"
)

// seedInvigilationFixtures gives a semester one room and one planned exam, so a
// room duty has a duration to inherit.
//
// The assembled-exam cache has to be warm for that: AddInvigilationAt takes the
// duration from ExamsAt, which reads the cache and skips any exam that is not in
// it. That is the Mongo behaviour too -- getMaxDurationForRoomAt discards the
// error (`examsInSlot, _ := db.ExamsAt(...)`), so a cold cache silently yields
// duration 0 rather than failing.
func seedInvigilationFixtures(t *testing.T, pg *db.PG, start time.Time) {
	t.Helper()
	ctx := t.Context()

	seedExamFixtures(t, pg, 100)
	exec(t, pg, `insert into room (name, seats) values ('R1.046', 60), ('R1.006', 30)`)
	if _, err := pg.AddExamToSlot(ctx, &model.PlanEntry{Ancode: 100, Starttime: &start}); err != nil {
		t.Fatalf("AddExamToSlot: %v", err)
	}
	if err := pg.ReplacePlannedRooms(ctx, []*model.PlannedRoom{
		{Ancode: 100, RoomName: "R1.046", Duration: 90},
		{Ancode: 100, RoomName: "R1.006", Duration: 120},
	}); err != nil {
		t.Fatalf("ReplacePlannedRooms: %v", err)
	}
	if err := pg.CacheAssembledExams(ctx, []*model.AssembledExam{
		{Ancode: 100, MaxDuration: 120},
	}); err != nil {
		t.Fatalf("CacheAssembledExams: %v", err)
	}
}

// TestAddInvigilationAtMovesTheDuty pins the upserting ReplaceOne: assigning a
// second invigilator to the same room and slot replaces the first rather than
// adding a second duty -- GetInvigilatorAt treats two as an error.
func TestAddInvigilationAtMovesTheDuty(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	start := berlin(t, "2027-01-20 08:30")
	seedInvigilationFixtures(t, pg, start)

	if err := pg.AddInvigilationAt(ctx, "R1.046", start, 180); err != nil {
		t.Fatalf("AddInvigilationAt: %v", err)
	}
	if err := pg.AddInvigilationAt(ctx, "R1.046", start, 181); err != nil {
		t.Fatalf("AddInvigilationAt again: %v", err)
	}

	invigilations, err := pg.GetInvigilationsAt(ctx, "R1.046", start)
	if err != nil {
		t.Fatalf("GetInvigilationsAt: %v", err)
	}
	if len(invigilations) != 1 {
		t.Fatalf("got %d invigilations, want 1 -- the second assignment must move the duty",
			len(invigilations))
	}
	if invigilations[0].InvigilatorID != 181 {
		t.Errorf("invigilator = %d, want 181", invigilations[0].InvigilatorID)
	}
	// The duration is the room's longest planned use in that slot.
	if invigilations[0].Duration != 90 {
		t.Errorf("Duration = %d, want 90 (the room's block)", invigilations[0].Duration)
	}
	// Slot is derived on read, never stored.
	if invigilations[0].Slot == nil || !invigilations[0].Slot.Starttime.Equal(start) {
		t.Errorf("Slot = %#v, want it derived from the start time", invigilations[0].Slot)
	}
}

// The reserve is a duty with no room, and it must not collide with a room duty
// in the same slot -- the Mongo filters spelled that out as two predicates.
func TestTheReserveIsItsOwnDuty(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	start := berlin(t, "2027-01-20 08:30")
	seedInvigilationFixtures(t, pg, start)

	if err := pg.AddInvigilationAt(ctx, "R1.046", start, 180); err != nil {
		t.Fatalf("AddInvigilationAt(room): %v", err)
	}
	if err := pg.AddInvigilationAt(ctx, "reserve", start, 181); err != nil {
		t.Fatalf("AddInvigilationAt(reserve): %v", err)
	}

	reserve, err := pg.GetInvigilationsAt(ctx, "reserve", start)
	if err != nil {
		t.Fatalf("GetInvigilationsAt(reserve): %v", err)
	}
	if len(reserve) != 1 || reserve[0].InvigilatorID != 181 {
		t.Fatalf("reserve = %#v, want only invigilator 181", reserve)
	}
	if !reserve[0].IsReserve || reserve[0].RoomName != nil {
		t.Errorf("reserve duty = %#v, want no room and the reserve flag", reserve[0])
	}
	// The reserve gets the slot's longest block across all rooms, not one room's.
	if reserve[0].Duration != 120 {
		t.Errorf("Duration = %d, want 120 (the longest room in the slot)", reserve[0].Duration)
	}

	room, err := pg.GetInvigilationsAt(ctx, "R1.046", start)
	if err != nil {
		t.Fatalf("GetInvigilationsAt(room): %v", err)
	}
	if len(room) != 1 || room[0].InvigilatorID != 180 {
		t.Errorf("the reserve assignment disturbed the room duty: %#v", room)
	}
}

// Self and generated invigilations are one table told apart by a flag. Every
// read that used to concatenate two collections must still see both, and the
// ones that asked for one collection must still see one.
func TestSelfAndGeneratedInvigilationsStayApart(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	start := berlin(t, "2027-01-20 08:30")
	seedInvigilationFixtures(t, pg, start)

	if err := pg.AddInvigilationAt(ctx, "R1.046", start, 180); err != nil {
		t.Fatalf("AddInvigilationAt: %v", err)
	}
	room := "R1.006"
	if err := pg.ReplaceAll(ctx, db.TargetSelfInvigilations, []interface{}{
		&model.Invigilation{InvigilatorID: 181, Starttime: &start, RoomName: &room, Duration: 0},
	}); err != nil {
		t.Fatalf("ReplaceAll(self): %v", err)
	}

	all, err := pg.GetAllInvigilations(ctx)
	if err != nil {
		t.Fatalf("GetAllInvigilations: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("GetAllInvigilations = %d, want 2", len(all))
	}

	self, err := pg.GetSelfInvigilations(ctx)
	if err != nil {
		t.Fatalf("GetSelfInvigilations: %v", err)
	}
	if len(self) != 1 || !self[0].IsSelfInvigilation {
		t.Errorf("GetSelfInvigilations = %#v, want the one self duty", self)
	}

	other, err := pg.GetOtherInvigilations(ctx)
	if err != nil {
		t.Fatalf("GetOtherInvigilations: %v", err)
	}
	if len(other) != 1 || other[0].IsSelfInvigilation {
		t.Errorf("GetOtherInvigilations = %#v, want the one generated duty", other)
	}

	forInvigilator, err := pg.InvigilationsForInvigilator(ctx, 181)
	if err != nil {
		t.Fatalf("InvigilationsForInvigilator: %v", err)
	}
	if len(forInvigilator) != 1 {
		t.Errorf("InvigilationsForInvigilator(181) = %d, want 1 -- self duties count here",
			len(forInvigilator))
	}

	// Resetting the generated ones leaves the self-invigilations alone.
	if err := pg.ResetGeneratedInvigilations(ctx); err != nil {
		t.Fatalf("ResetGeneratedInvigilations: %v", err)
	}
	all, err = pg.GetAllInvigilations(ctx)
	if err != nil {
		t.Fatalf("GetAllInvigilations: %v", err)
	}
	if len(all) != 1 || !all[0].IsSelfInvigilation {
		t.Errorf("after the reset: %#v, want only the self duty", all)
	}
}

func TestSetInvigilationPrePlannedAt(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	start := berlin(t, "2027-01-20 08:30")
	seedInvigilationFixtures(t, pg, start)

	if err := pg.AddInvigilationAt(ctx, "R1.046", start, 180); err != nil {
		t.Fatalf("AddInvigilationAt: %v", err)
	}
	room := "R1.046"
	if err := pg.SetInvigilationPrePlannedAt(ctx, start, &room, true); err != nil {
		t.Fatalf("SetInvigilationPrePlannedAt: %v", err)
	}

	invigilations, err := pg.GetInvigilationsAt(ctx, "R1.046", start)
	if err != nil {
		t.Fatalf("GetInvigilationsAt: %v", err)
	}
	if !invigilations[0].PrePlanned {
		t.Error("the prePlanned flag was not set")
	}

	// Nothing to mark is an error, as before.
	other := "R1.006"
	if err := pg.SetInvigilationPrePlannedAt(ctx, start, &other, true); err == nil {
		t.Error("marking an invigilation that does not exist did not fail")
	}
	// And the reserve is addressed with a nil room, not with the room's name.
	if err := pg.SetInvigilationPrePlannedAt(ctx, start, nil, true); err == nil {
		t.Error("marking a reserve that does not exist did not fail")
	}
}

// TestPrePlanningIsKeyedByRoomAndSlot is the schema fix. Both methods address a
// pre-planned duty by (starttime, roomName) -- "only one invigilator per room
// (or reserve) at a time". A key on the invigilator would have let a second
// person be added to the same room and slot instead of replacing the first.
func TestPrePlanningIsKeyedByRoomAndSlot(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	start := berlin(t, "2027-01-20 08:30")
	seedInvigilationFixtures(t, pg, start)
	room := "R1.046"

	if ok, err := pg.AddPrePlannedInvigilation(ctx, &model.PrePlannedInvigilation{
		InvigilatorID: 180, Starttime: &start, RoomName: &room,
	}); err != nil || !ok {
		t.Fatalf("AddPrePlannedInvigilation: %v, ok=%v", err, ok)
	}
	if ok, err := pg.AddPrePlannedInvigilation(ctx, &model.PrePlannedInvigilation{
		InvigilatorID: 181, Starttime: &start, RoomName: &room,
	}); err != nil || !ok {
		t.Fatalf("AddPrePlannedInvigilation again: %v, ok=%v", err, ok)
	}

	invigilations, err := pg.PrePlannedInvigilations(ctx)
	if err != nil {
		t.Fatalf("PrePlannedInvigilations: %v", err)
	}
	if len(invigilations) != 1 {
		t.Fatalf("got %d pre-planned duties, want 1 -- one invigilator per room and slot",
			len(invigilations))
	}
	if invigilations[0].InvigilatorID != 181 {
		t.Errorf("invigilator = %d, want 181 (the second assignment replaces the first)",
			invigilations[0].InvigilatorID)
	}

	// One person may still cover a second room in the same slot -- which a key on
	// (invigilator, starttime) would have forbidden.
	otherRoom := "R1.006"
	if _, err := pg.AddPrePlannedInvigilation(ctx, &model.PrePlannedInvigilation{
		InvigilatorID: 181, Starttime: &start, RoomName: &otherRoom,
	}); err != nil {
		t.Fatalf("a second room for the same invigilator and slot was rejected: %v", err)
	}
	// As may the reserve, which is the NULL room.
	if _, err := pg.AddPrePlannedInvigilation(ctx, &model.PrePlannedInvigilation{
		InvigilatorID: 182, Starttime: &start, IsReserve: true,
	}); err != nil {
		t.Fatalf("AddPrePlannedInvigilation(reserve): %v", err)
	}

	forInvigilator, err := pg.PrePlannedInvigilationsForInvigilator(ctx, 181)
	if err != nil {
		t.Fatalf("PrePlannedInvigilationsForInvigilator: %v", err)
	}
	if len(forInvigilator) != 2 {
		t.Errorf("got %d duties for invigilator 181, want 2", len(forInvigilator))
	}

	removed, err := pg.RemovePrePlannedInvigilationAt(ctx, start, nil)
	if err != nil {
		t.Fatalf("RemovePrePlannedInvigilationAt: %v", err)
	}
	if !removed {
		t.Error("the reserve was not removed by a nil room name")
	}
	removed, err = pg.RemovePrePlannedInvigilationAt(ctx, start, nil)
	if err != nil {
		t.Fatalf("RemovePrePlannedInvigilationAt: %v", err)
	}
	if removed {
		t.Error("RemovePrePlannedInvigilationAt reported a removal that did not happen")
	}
}

// A pre-planned duty without a start time is refused: the absolute time is the
// source of truth and half of the key.
func TestPrePlannedInvigilationNeedsATime(t *testing.T) {
	pg := pgtest.NewDB(t)
	seedSemester(t, pg)

	if _, err := pg.AddPrePlannedInvigilation(t.Context(),
		&model.PrePlannedInvigilation{InvigilatorID: 180}); err == nil {
		t.Error("a pre-planned invigilation without a start time was accepted")
	}
}

func TestInvigilatorRequirementsAndTodos(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()
	seedSemester(t, pg)

	// Nothing stored yet.
	if req, err := pg.GetInvigilatorRequirements(ctx, 180); err != nil {
		t.Fatalf("GetInvigilatorRequirements: %v", err)
	} else if req != nil {
		t.Errorf("GetInvigilatorRequirements = %#v, want nil", req)
	}
	if todos, err := pg.GetInvigilationTodos(ctx); err != nil {
		t.Fatalf("GetInvigilationTodos: %v", err)
	} else if todos != nil {
		t.Errorf("GetInvigilationTodos = %#v, want nil", todos)
	}

	if err := pg.ReplaceAll(ctx, db.TargetInvigilatorRequirements, []interface{}{
		&zpa.SupervisorRequirements{
			Invigilator: "Braun", InvigilatorID: 180, PartTime: 1,
			ExcludedDates: []string{"20.01.27"},
		},
	}); err != nil {
		t.Fatalf("ReplaceAll: %v", err)
	}

	req, err := pg.GetInvigilatorRequirements(ctx, 180)
	if err != nil {
		t.Fatalf("GetInvigilatorRequirements: %v", err)
	}
	if req == nil || req.Invigilator != "Braun" {
		t.Fatalf("requirements = %#v, want Braun", req)
	}
	// The excluded dates stay ZPA's own strings, parsed on read.
	if len(req.ExcludedDates) != 1 || req.ExcludedDates[0] != "20.01.27" {
		t.Errorf("ExcludedDates = %v, want the ZPA string back", req.ExcludedDates)
	}

	all, err := pg.AllInvigilatorRequirements(ctx)
	if err != nil {
		t.Fatalf("AllInvigilatorRequirements: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("AllInvigilatorRequirements = %d, want 1", len(all))
	}

	// The todos are one row per semester -- caching twice cannot leave two behind,
	// which is what the fixed _id and the healing DeleteMany were arranging.
	todos := &model.InvigilationTodos{SumExamRooms: 4200, InvigilatorCount: 30}
	if err := pg.CacheInvigilatorTodos(ctx, todos); err != nil {
		t.Fatalf("CacheInvigilatorTodos: %v", err)
	}
	todos.SumExamRooms = 4300
	if err := pg.CacheInvigilatorTodos(ctx, todos); err != nil {
		t.Fatalf("CacheInvigilatorTodos again: %v", err)
	}
	if n := count(t, pg, `select count(*) from invigilation_todos where semester_id='2026-WS'`); n != 1 {
		t.Errorf("invigilation_todos rows = %d, want 1", n)
	}

	got, err := pg.GetInvigilationTodos(ctx)
	if err != nil {
		t.Fatalf("GetInvigilationTodos: %v", err)
	}
	if got == nil || got.SumExamRooms != 4300 {
		t.Errorf("todos = %#v, want the second write", got)
	}
}

func TestInvigilatorConstraintsRoundTrip(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()
	seedSemester(t, pg)

	day := berlin(t, "2027-01-20 00:00")
	from := berlin(t, "2027-01-20 10:00")
	until := berlin(t, "2027-01-20 14:00")

	if err := pg.UpsertInvigilatorConstraints(ctx, &model.InvigilatorConstraints{
		TeacherID:        180,
		IsNotInvigilator: false,
		ExcludedDates:    []time.Time{day},
		TimeWindows: []*model.InvigilationTimeWindow{
			{Date: day, From: &from, Until: &until},
		},
	}); err != nil {
		t.Fatalf("UpsertInvigilatorConstraints: %v", err)
	}

	got, err := pg.InvigilatorConstraintsForTeacher(ctx, 180)
	if err != nil {
		t.Fatalf("InvigilatorConstraintsForTeacher: %v", err)
	}
	if got == nil {
		t.Fatal("InvigilatorConstraintsForTeacher = nil")
	}
	if len(got.ExcludedDates) != 1 || got.ExcludedDates[0] != day {
		t.Errorf("ExcludedDates = %v, want the struct-identical day back", got.ExcludedDates)
	}
	if len(got.TimeWindows) != 1 || got.TimeWindows[0].From == nil ||
		!got.TimeWindows[0].From.Equal(from) {
		t.Errorf("TimeWindows = %#v, want the window back", got.TimeWindows)
	}

	// Replacing the record replaces its dates and windows.
	if err := pg.UpsertInvigilatorConstraints(ctx, &model.InvigilatorConstraints{
		TeacherID: 180, IsNotInvigilator: true,
	}); err != nil {
		t.Fatalf("UpsertInvigilatorConstraints: %v", err)
	}
	got, err = pg.InvigilatorConstraintsForTeacher(ctx, 180)
	if err != nil {
		t.Fatalf("InvigilatorConstraintsForTeacher: %v", err)
	}
	if !got.IsNotInvigilator || len(got.ExcludedDates) != 0 || len(got.TimeWindows) != 0 {
		t.Errorf("constraints after the replace = %#v", got)
	}
	// Empty lists, not nil -- the GraphQL fields are non-null.
	if got.ExcludedDates == nil || got.TimeWindows == nil {
		t.Error("empty lists came back as nil")
	}

	all, err := pg.InvigilatorConstraints(ctx)
	if err != nil {
		t.Fatalf("InvigilatorConstraints: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("InvigilatorConstraints = %d, want 1", len(all))
	}

	if removed, err := pg.DeleteInvigilatorConstraints(ctx, 180); err != nil || !removed {
		t.Fatalf("DeleteInvigilatorConstraints: %v, removed=%v", err, removed)
	}
	for _, table := range []string{"invigilator_excluded_date", "invigilator_time_window"} {
		if n := count(t, pg, `select count(*) from `+table+` where semester_id='2026-WS'`); n != 0 {
			t.Errorf("%s still has %d rows -- they must cascade", table, n)
		}
	}
	if removed, err := pg.DeleteInvigilatorConstraints(ctx, 180); err != nil || removed {
		t.Errorf("DeleteInvigilatorConstraints again: %v, removed=%v, want false", err, removed)
	}

	// A teacher with no constraints is (nil, nil), never an error.
	if got, err := pg.InvigilatorConstraintsForTeacher(ctx, 999); err != nil || got != nil {
		t.Errorf("InvigilatorConstraintsForTeacher(999) = %#v, %v, want nil, nil", got, err)
	}
}
