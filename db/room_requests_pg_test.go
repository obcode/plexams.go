package db_test

import (
	"testing"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
)

func testRoomRequest(room string, starttime time.Time) *model.RoomRequest {
	// valid_from 15 minutes before the slot, as the generator produces it.
	from := starttime.Add(-15 * time.Minute)
	return &model.RoomRequest{
		Room:      room,
		Starttime: &starttime,
		From:      from,
		Until:     from.Add(2 * time.Hour),
		Active:    true,
	}
}

// TestARoomRequestStaysAddressableAfterItsWindowMoves is the schema fix: the key
// is (room, starttime), not (room, valid_from). UpdateRoomRequestTime changes
// valid_from, so a key containing it would move the row out from under the
// caller that just edited it -- and the request could never be approved again.
func TestARoomRequestStaysAddressableAfterItsWindowMoves(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	exec(t, pg, `insert into semester (id, semester, schema_version)
	             values ('2026-WS', '2026 WS', 2)`)
	exec(t, pg, `insert into room (name, seats) values ('R1.006', 30)`)
	start := berlin(t, "2027-01-20 08:30")

	if err := pg.AddRoomRequest(ctx, testRoomRequest("R1.006", start)); err != nil {
		t.Fatalf("AddRoomRequest: %v", err)
	}

	// Extend it for an NTA -- the real reason this method exists.
	moved, err := pg.UpdateRoomRequestTime(ctx, "R1.006", start,
		start.Add(-30*time.Minute), start.Add(3*time.Hour))
	if err != nil {
		t.Fatalf("UpdateRoomRequestTime: %v", err)
	}
	if moved == nil {
		t.Fatal("UpdateRoomRequestTime found nothing to update")
	}

	got, err := pg.GetRoomRequest(ctx, "R1.006", start)
	if err != nil {
		t.Fatalf("GetRoomRequest: %v", err)
	}
	if got == nil {
		t.Fatal("the request is unreachable after its window moved")
	}
	if !got.From.Equal(start.Add(-30 * time.Minute)) {
		t.Errorf("From = %v, want the new window start", got.From)
	}

	approved, err := pg.SetRoomRequestApproved(ctx, "R1.006", start, true)
	if err != nil {
		t.Fatalf("SetRoomRequestApproved: %v", err)
	}
	if approved == nil || !approved.Approved {
		t.Errorf("SetRoomRequestApproved = %#v, want an approved request", approved)
	}
}

// A request that does not exist is (nil, nil) everywhere, never an error --
// plexams.AddRoomRequest reads the nil as "does not exist yet".
func TestMissingRoomRequestIsNil(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	exec(t, pg, `insert into semester (id, semester, schema_version)
	             values ('2026-WS', '2026 WS', 2)`)
	exec(t, pg, `insert into room (name, seats) values ('R1.006', 30)`)
	start := berlin(t, "2027-01-20 08:30")

	for _, tc := range []struct {
		name string
		call func() (*model.RoomRequest, error)
	}{
		{"GetRoomRequest", func() (*model.RoomRequest, error) {
			return pg.GetRoomRequest(ctx, "R1.006", start)
		}},
		{"SetRoomRequestApproved", func() (*model.RoomRequest, error) {
			return pg.SetRoomRequestApproved(ctx, "R1.006", start, true)
		}},
		{"SetRoomRequestActive", func() (*model.RoomRequest, error) {
			return pg.SetRoomRequestActive(ctx, "R1.006", start, false)
		}},
		{"UpdateRoomRequestTime", func() (*model.RoomRequest, error) {
			return pg.UpdateRoomRequestTime(ctx, "R1.006", start, start, start.Add(time.Hour))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.call()
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if got != nil {
				t.Errorf("%s = %#v, want nil", tc.name, got)
			}
		})
	}
}

func TestReplaceAllRoomRequests(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	exec(t, pg, `insert into semester (id, semester, schema_version)
	             values ('2026-WS', '2026 WS', 2)`)
	exec(t, pg, `insert into room (name, seats) values ('R1.006', 30), ('R1.046', 60)`)
	start := berlin(t, "2027-01-20 08:30")

	if err := pg.AddRoomRequest(ctx, testRoomRequest("R1.046", start)); err != nil {
		t.Fatalf("AddRoomRequest: %v", err)
	}
	if err := pg.ReplaceAllRoomRequests(ctx, []*model.RoomRequest{
		testRoomRequest("R1.006", start),
		testRoomRequest("R1.006", start.Add(2*time.Hour)),
	}); err != nil {
		t.Fatalf("ReplaceAllRoomRequests: %v", err)
	}

	requests, err := pg.RoomRequests(ctx)
	if err != nil {
		t.Fatalf("RoomRequests: %v", err)
	}
	if len(requests) != 2 {
		t.Fatalf("got %d requests, want 2", len(requests))
	}
	for _, request := range requests {
		if request.Room != "R1.006" {
			t.Errorf("room %s survived the replace", request.Room)
		}
	}

	// An empty replace clears them -- the Mongo version dropped the collection
	// and returned early.
	if err := pg.ReplaceAllRoomRequests(ctx, nil); err != nil {
		t.Fatalf("ReplaceAllRoomRequests(nil): %v", err)
	}
	if requests, err := pg.RoomRequests(ctx); err != nil {
		t.Fatalf("RoomRequests: %v", err)
	} else if len(requests) != 0 {
		t.Errorf("%d requests left after an empty replace", len(requests))
	}
}

func TestBlockAndUnblockRoom(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	exec(t, pg, `insert into semester (id, semester, schema_version)
	             values ('2026-WS', '2026 WS', 2)`)
	exec(t, pg, `insert into room (name, seats) values ('R1.008', 24)`)
	start := berlin(t, "2027-01-20 08:30")
	reason := "Werkstatt"

	if err := pg.BlockRoomForSlot(ctx, &model.BlockedRoom{
		Room: "R1.008", Starttime: &start,
	}); err != nil {
		t.Fatalf("BlockRoomForSlot: %v", err)
	}
	// Blocking the same room and slot again replaces the block, as ReplaceOne
	// with upsert did -- it does not create a second one.
	if err := pg.BlockRoomForSlot(ctx, &model.BlockedRoom{
		Room: "R1.008", Starttime: &start, Reason: &reason,
	}); err != nil {
		t.Fatalf("BlockRoomForSlot again: %v", err)
	}

	blocked, err := pg.BlockedRooms(ctx)
	if err != nil {
		t.Fatalf("BlockedRooms: %v", err)
	}
	if len(blocked) != 1 {
		t.Fatalf("got %d blocks, want 1", len(blocked))
	}
	if blocked[0].Reason == nil || *blocked[0].Reason != reason {
		t.Errorf("Reason = %v, want %q", blocked[0].Reason, reason)
	}
	if !blocked[0].Starttime.Equal(start) {
		t.Errorf("Starttime = %v, want %v", blocked[0].Starttime, start)
	}

	removed, err := pg.UnblockRoomForSlot(ctx, "R1.008", start)
	if err != nil {
		t.Fatalf("UnblockRoomForSlot: %v", err)
	}
	if !removed {
		t.Error("UnblockRoomForSlot reported nothing removed")
	}
	removed, err = pg.UnblockRoomForSlot(ctx, "R1.008", start)
	if err != nil {
		t.Fatalf("UnblockRoomForSlot: %v", err)
	}
	if removed {
		t.Error("UnblockRoomForSlot reported a removal that did not happen")
	}
}

// A block without a start time is refused rather than stored: nothing could
// address it again, and BlockRoomForSlot's contract is that the caller supplies
// the slot's start time.
func TestBlockingARoomWithoutATimeFails(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	exec(t, pg, `insert into semester (id, semester, schema_version)
	             values ('2026-WS', '2026 WS', 2)`)
	exec(t, pg, `insert into room (name, seats) values ('R1.008', 24)`)

	if err := pg.BlockRoomForSlot(ctx, &model.BlockedRoom{Room: "R1.008"}); err == nil {
		t.Error("a block without a starttime was accepted")
	}
}

func TestUnplacedExamsReadTheirTimeFromThePlan(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg, 225)
	start := berlin(t, "2027-01-20 08:30")
	if _, err := pg.AddExamToSlot(ctx, &model.PlanEntry{Ancode: 225, Starttime: &start}); err != nil {
		t.Fatalf("AddExamToSlot: %v", err)
	}

	if err := pg.ReplaceUnplacedExams(ctx, []*model.UnplacedExam{
		{Ancode: 225, Mtknrs: []string{"00000001", "00000002"}},
	}); err != nil {
		t.Fatalf("ReplaceUnplacedExams: %v", err)
	}

	unplaced, err := pg.UnplacedExams(ctx)
	if err != nil {
		t.Fatalf("UnplacedExams: %v", err)
	}
	if len(unplaced) != 1 {
		t.Fatalf("got %d unplaced exams, want 1", len(unplaced))
	}
	if unplaced[0].Starttime == nil || !unplaced[0].Starttime.Equal(start) {
		t.Errorf("Starttime = %v, want %v -- it is joined from the plan entry",
			unplaced[0].Starttime, start)
	}
	if len(unplaced[0].Mtknrs) != 2 {
		t.Errorf("Mtknrs = %v, want two students", unplaced[0].Mtknrs)
	}

	if err := pg.ResetUnplacedExams(ctx); err != nil {
		t.Fatalf("ResetUnplacedExams: %v", err)
	}
	if unplaced, err := pg.UnplacedExams(ctx); err != nil {
		t.Fatalf("UnplacedExams: %v", err)
	} else if len(unplaced) != 0 {
		t.Errorf("%d unplaced exams left after the reset", len(unplaced))
	}
}

// An exam with no unplaced students at all still round-trips with an empty
// slice, not nil -- the GraphQL list is non-null.
func TestUnplacedExamWithoutStudentsHasAnEmptyList(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg, 225)
	start := berlin(t, "2027-01-20 08:30")
	if _, err := pg.AddExamToSlot(ctx, &model.PlanEntry{Ancode: 225, Starttime: &start}); err != nil {
		t.Fatalf("AddExamToSlot: %v", err)
	}
	if err := pg.ReplaceUnplacedExams(ctx, []*model.UnplacedExam{{Ancode: 225}}); err != nil {
		t.Fatalf("ReplaceUnplacedExams: %v", err)
	}

	unplaced, err := pg.UnplacedExams(ctx)
	if err != nil {
		t.Fatalf("UnplacedExams: %v", err)
	}
	if len(unplaced) != 1 || unplaced[0].Mtknrs == nil {
		t.Errorf("Mtknrs = %v, want an empty slice", unplaced[0].Mtknrs)
	}
}
