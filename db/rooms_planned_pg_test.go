package db_test

import (
	"testing"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
)

func ptrStr(s string) *string { return &s }

// TestPlannedRoomsReadTheirTimeFromThePlan is the room-plan half of the point of
// the migration: the start time comes back from the exam's plan entry, so moving
// the exam moves its rooms with it.
func TestPlannedRoomsReadTheirTimeFromThePlan(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg, 225)
	exec(t, pg, `insert into room (name, seats) values ('R1.046', 60)`)
	first := berlin(t, "2027-01-20 08:30")
	second := berlin(t, "2027-01-22 14:00")

	if _, err := pg.AddExamToSlot(ctx, &model.PlanEntry{Ancode: 225, Starttime: &first}); err != nil {
		t.Fatalf("AddExamToSlot: %v", err)
	}
	if err := pg.ReplacePlannedRooms(ctx, []*model.PlannedRoom{
		{Ancode: 225, RoomName: "R1.046", Duration: 90, StudentsInRoom: []string{"00000002", "00000001"}},
	}); err != nil {
		t.Fatalf("ReplacePlannedRooms: %v", err)
	}

	rooms, err := pg.PlannedRoomsAt(ctx, first)
	if err != nil {
		t.Fatalf("PlannedRoomsAt: %v", err)
	}
	if len(rooms) != 1 {
		t.Fatalf("PlannedRoomsAt(first) = %d rooms, want 1", len(rooms))
	}

	// Move the exam. Only the plan entry is touched -- exactly what SetExamTime
	// does, and exactly what used to leave the room plan behind.
	if _, err := pg.AddExamToSlot(ctx, &model.PlanEntry{Ancode: 225, Starttime: &second}); err != nil {
		t.Fatalf("AddExamToSlot (move): %v", err)
	}

	if rooms, err := pg.PlannedRoomsAt(ctx, first); err != nil {
		t.Fatalf("PlannedRoomsAt: %v", err)
	} else if len(rooms) != 0 {
		t.Errorf("%d rooms still at the old time -- the room plan went stale", len(rooms))
	}

	rooms, err = pg.PlannedRoomsAt(ctx, second)
	if err != nil {
		t.Fatalf("PlannedRoomsAt: %v", err)
	}
	if len(rooms) != 1 {
		t.Fatalf("PlannedRoomsAt(second) = %d rooms, want 1", len(rooms))
	}
	if !rooms[0].Starttime.Equal(second) {
		t.Errorf("Starttime = %v, want %v", rooms[0].Starttime, second)
	}
	if got := rooms[0].StudentsInRoom; len(got) != 2 || got[0] != "00000001" {
		t.Errorf("StudentsInRoom = %v, want both students, sorted", got)
	}
}

// A room with no students comes back with an empty slice, not nil: the GraphQL
// list is non-null, and 2026-SS really does contain three such rooms.
func TestAnEmptyRoomHasAnEmptyStudentList(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg, 225)
	exec(t, pg, `insert into room (name, seats) values ('R1.046', 60)`)
	start := berlin(t, "2027-01-20 08:30")
	if _, err := pg.AddExamToSlot(ctx, &model.PlanEntry{Ancode: 225, Starttime: &start}); err != nil {
		t.Fatalf("AddExamToSlot: %v", err)
	}
	if err := pg.ReplacePlannedRooms(ctx, []*model.PlannedRoom{
		{Ancode: 225, RoomName: "R1.046", Duration: 90, Reserve: true},
	}); err != nil {
		t.Fatalf("ReplacePlannedRooms: %v", err)
	}

	rooms, err := pg.PlannedRoomsForAncode(ctx, 225)
	if err != nil {
		t.Fatalf("PlannedRoomsForAncode: %v", err)
	}
	if len(rooms) != 1 {
		t.Fatalf("got %d rooms, want 1", len(rooms))
	}
	if rooms[0].StudentsInRoom == nil {
		t.Error("StudentsInRoom is nil, want an empty slice")
	}
	if len(rooms[0].StudentsInRoom) != 0 {
		t.Errorf("StudentsInRoom = %v, want empty", rooms[0].StudentsInRoom)
	}
}

// A duration of 0 is real data, not a defect: the room generator copies the
// exam's MaxDuration verbatim, and 2026-SS has a pre-planned room for an exam
// whose duration nobody set. A `duration_min > 0` check would have made room
// generation fail on it.
func TestARoomMayHaveNoDuration(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg, 543)
	exec(t, pg, `insert into room (name, seats) values ('R2.007', 40)`)
	start := berlin(t, "2027-01-20 08:30")
	if _, err := pg.AddExamToSlot(ctx, &model.PlanEntry{Ancode: 543, Starttime: &start}); err != nil {
		t.Fatalf("AddExamToSlot: %v", err)
	}

	if err := pg.ReplacePlannedRooms(ctx, []*model.PlannedRoom{
		{Ancode: 543, RoomName: "R2.007", Duration: 0, PrePlanned: true,
			StudentsInRoom: []string{"16934526"}},
	}); err != nil {
		t.Fatalf("ReplacePlannedRooms with duration 0: %v", err)
	}
}

// Several bookings of one room for one exam, one per NTA student, plus the
// ordinary one. The natural key is (ancode, room, nta_mtknr).
func TestSeveralNTABookingsOfOneRoomSurviveAReplace(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg, 225)
	exec(t, pg, `insert into room (name, seats) values ('R1.046', 60)`)
	exec(t, pg, `insert into nta (mtknr, name, compensation, delta_duration_percent,
	                              program, valid_from, valid_until)
	             values ('39644321', 'A', 'Zeitverlängerung', 10, 'IF-B', '2026-WS', '2027-SS'),
	                    ('21384524', 'B', 'Zeitverlängerung', 10, 'IF-B', '2026-WS', '2027-SS')`)
	start := berlin(t, "2027-01-20 08:30")
	if _, err := pg.AddExamToSlot(ctx, &model.PlanEntry{Ancode: 225, Starttime: &start}); err != nil {
		t.Fatalf("AddExamToSlot: %v", err)
	}

	if err := pg.ReplacePlannedRooms(ctx, []*model.PlannedRoom{
		{Ancode: 225, RoomName: "R1.046", Duration: 90, StudentsInRoom: []string{"00000001"}},
		{Ancode: 225, RoomName: "R1.046", Duration: 99, Handicap: true,
			NtaMtknr: ptrStr("39644321"), StudentsInRoom: []string{"39644321"}},
		{Ancode: 225, RoomName: "R1.046", Duration: 99, Handicap: true,
			NtaMtknr: ptrStr("21384524"), StudentsInRoom: []string{"21384524"}},
	}); err != nil {
		t.Fatalf("ReplacePlannedRooms: %v", err)
	}

	rooms, err := pg.PlannedRoomsForAncode(ctx, 225)
	if err != nil {
		t.Fatalf("PlannedRoomsForAncode: %v", err)
	}
	if len(rooms) != 3 {
		t.Fatalf("got %d bookings, want 3", len(rooms))
	}

	names, err := pg.PlannedRoomNames(ctx)
	if err != nil {
		t.Fatalf("PlannedRoomNames: %v", err)
	}
	if len(names) != 1 || names[0] != "R1.046" {
		t.Errorf("PlannedRoomNames = %v, want one distinct name", names)
	}

	namesAt, err := pg.PlannedRoomNamesAt(ctx, start)
	if err != nil {
		t.Fatalf("PlannedRoomNamesAt: %v", err)
	}
	if len(namesAt) != 1 {
		t.Errorf("PlannedRoomNamesAt = %v, want one distinct name", namesAt)
	}
}

// ReplacePlannedRooms is all-or-nothing. A room for an exam that is not in the
// plan is rejected, and the previous plan must still be there afterwards --
// under MongoDB a failing insert left the collection empty.
func TestAFailingReplaceLeavesTheRoomPlanIntact(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg, 225, 226)
	exec(t, pg, `insert into room (name, seats) values ('R1.046', 60)`)
	start := berlin(t, "2027-01-20 08:30")
	if _, err := pg.AddExamToSlot(ctx, &model.PlanEntry{Ancode: 225, Starttime: &start}); err != nil {
		t.Fatalf("AddExamToSlot: %v", err)
	}
	if err := pg.ReplacePlannedRooms(ctx, []*model.PlannedRoom{
		{Ancode: 225, RoomName: "R1.046", Duration: 90, StudentsInRoom: []string{"00000001"}},
	}); err != nil {
		t.Fatalf("ReplacePlannedRooms: %v", err)
	}

	// Ancode 226 exists but is not in the plan -- the FK rejects it.
	err := pg.ReplacePlannedRooms(ctx, []*model.PlannedRoom{
		{Ancode: 225, RoomName: "R1.046", Duration: 90},
		{Ancode: 226, RoomName: "R1.046", Duration: 90},
	})
	if err == nil {
		t.Fatal("a room for an unplanned exam was accepted")
	}

	rooms, err := pg.PlannedRooms(ctx)
	if err != nil {
		t.Fatalf("PlannedRooms: %v", err)
	}
	if len(rooms) != 1 || len(rooms[0].StudentsInRoom) != 1 {
		t.Errorf("room plan after the failed replace = %+v, want the previous one", rooms)
	}
}

// ResetPlannedRooms clears the generated plan and leaves the pre-planning alone.
func TestResetPlannedRoomsKeepsThePrePlanning(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg, 225)
	exec(t, pg, `insert into room (name, seats) values ('R1.046', 60)`)
	start := berlin(t, "2027-01-20 08:30")
	if _, err := pg.AddExamToSlot(ctx, &model.PlanEntry{Ancode: 225, Starttime: &start}); err != nil {
		t.Fatalf("AddExamToSlot: %v", err)
	}
	if err := pg.ReplacePlannedRooms(ctx, []*model.PlannedRoom{
		{Ancode: 225, RoomName: "R1.046", Duration: 90},
	}); err != nil {
		t.Fatalf("ReplacePlannedRooms: %v", err)
	}
	if _, err := pg.AddPrePlannedRoomToExam(ctx, &model.PrePlannedRoom{
		Ancode: 225, RoomName: "R1.046",
	}); err != nil {
		t.Fatalf("AddPrePlannedRoomToExam: %v", err)
	}

	if err := pg.ResetPlannedRooms(ctx); err != nil {
		t.Fatalf("ResetPlannedRooms: %v", err)
	}

	if rooms, err := pg.PlannedRooms(ctx); err != nil {
		t.Fatalf("PlannedRooms: %v", err)
	} else if len(rooms) != 0 {
		t.Errorf("%d planned rooms left after the reset", len(rooms))
	}
	if rooms, err := pg.PrePlannedRooms(ctx); err != nil {
		t.Fatalf("PrePlannedRooms: %v", err)
	} else if len(rooms) != 1 {
		t.Errorf("%d pre-planned rooms left, want 1 -- the reset must not touch them", len(rooms))
	}
}

// TestPrePlanningIsKeyedByStudentToo is the schema bug the live data found: the
// planner pre-assigns a room for the exam as a whole (mtknr NULL) and pins
// individual NTA students into rooms besides. 2026-SS has ancode 355 four times
// in T3.017, so a key of (ancode, room) would have rejected the real room plan.
func TestPrePlanningIsKeyedByStudentToo(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg, 355)
	exec(t, pg, `insert into room (name, seats) values ('T3.017', 30)`)

	for _, mtknr := range []*string{nil, ptrStr("40834723"), ptrStr("39644321"), ptrStr("21906624")} {
		if _, err := pg.AddPrePlannedRoomToExam(ctx, &model.PrePlannedRoom{
			Ancode: 355, RoomName: "T3.017", Mtknr: mtknr,
		}); err != nil {
			t.Fatalf("AddPrePlannedRoomToExam(%v): %v", mtknr, err)
		}
	}

	rooms, err := pg.PrePlannedRoomsForExam(ctx, 355)
	if err != nil {
		t.Fatalf("PrePlannedRoomsForExam: %v", err)
	}
	if len(rooms) != 4 {
		t.Fatalf("got %d pre-planned rooms, want 4 (one per student plus the exam's own)", len(rooms))
	}

	// Adding the same (ancode, room, student) again replaces it -- that is what
	// the Mongo delete-then-insert did.
	if _, err := pg.AddPrePlannedRoomToExam(ctx, &model.PrePlannedRoom{
		Ancode: 355, RoomName: "T3.017", Mtknr: ptrStr("40834723"), Reserve: true,
	}); err != nil {
		t.Fatalf("AddPrePlannedRoomToExam again: %v", err)
	}
	rooms, err = pg.PrePlannedRoomsForExam(ctx, 355)
	if err != nil {
		t.Fatalf("PrePlannedRoomsForExam: %v", err)
	}
	if len(rooms) != 4 {
		t.Fatalf("got %d pre-planned rooms after the replace, want 4", len(rooms))
	}

	// Removing the room with no student must not take the students' rows with it.
	removed, err := pg.RemovePrePlannedRoomFromExam(ctx, 355, "T3.017", nil)
	if err != nil {
		t.Fatalf("RemovePrePlannedRoomFromExam: %v", err)
	}
	if !removed {
		t.Error("RemovePrePlannedRoomFromExam reported nothing removed")
	}
	rooms, err = pg.PrePlannedRoomsForExam(ctx, 355)
	if err != nil {
		t.Fatalf("PrePlannedRoomsForExam: %v", err)
	}
	if len(rooms) != 3 {
		t.Errorf("got %d pre-planned rooms after removing the exam's own, want 3", len(rooms))
	}

	// Removing something that is not there reports false, as DeleteOne did.
	removed, err = pg.RemovePrePlannedRoomFromExam(ctx, 355, "T3.017", ptrStr("00000000"))
	if err != nil {
		t.Fatalf("RemovePrePlannedRoomFromExam: %v", err)
	}
	if removed {
		t.Error("RemovePrePlannedRoomFromExam reported a removal that did not happen")
	}
}

// The pre-planning hangs off the exam, not the plan entry: it exists before
// anything is scheduled. That is the whole point of pre-planning.
func TestPrePlanningWorksBeforeAnythingIsScheduled(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg, 134)
	exec(t, pg, `insert into room (name, seats) values ('T3.017', 30)`)

	seats := 12
	if _, err := pg.AddPrePlannedRoomToExam(ctx, &model.PrePlannedRoom{
		Ancode: 134, RoomName: "T3.017", Seats: &seats,
	}); err != nil {
		t.Fatalf("AddPrePlannedRoomToExam: %v", err)
	}

	rooms, err := pg.PrePlannedRooms(ctx)
	if err != nil {
		t.Fatalf("PrePlannedRooms: %v", err)
	}
	if len(rooms) != 1 {
		t.Fatalf("got %d pre-planned rooms, want 1", len(rooms))
	}
	if rooms[0].Seats == nil || *rooms[0].Seats != 12 {
		t.Errorf("Seats = %v, want 12", rooms[0].Seats)
	}
	if rooms[0].Mtknr != nil {
		t.Errorf("Mtknr = %v, want nil", rooms[0].Mtknr)
	}
}
