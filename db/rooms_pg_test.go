package db_test

import (
	"strconv"
	"testing"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
)

func intptr(i int) *int { return &i }

func intval(i *int) string {
	if i == nil {
		return "<nil>"
	}
	return strconv.Itoa(*i)
}

func testRoom(name string) *model.Room {
	return &model.Room{
		Name:             name,
		Seats:            57,
		Handicap:         true,
		Lab:              false,
		PlacesWithSocket: true,
		RequestWith:      model.RoomRequestTypeManagement,
		RequestPriority:  1,
		Exahm:            false,
		Seb:              true,
		SebSeats:         intptr(30),
		HmebSeats:        intptr(12),
		Deactivated:      false,
		Hitzewert:        intptr(3),
	}
}

// assertRoomEqual compares field by field so a failure names the offending
// column. NeedsRequest is checked separately: it is derived, not carried over.
func assertRoomEqual(t *testing.T, want, got *model.Room) {
	t.Helper()

	if got == nil {
		t.Fatal("room is nil")
	}
	if got.Name != want.Name {
		t.Errorf("Name = %q, want %q", got.Name, want.Name)
	}
	if got.Seats != want.Seats {
		t.Errorf("Seats = %d, want %d", got.Seats, want.Seats)
	}
	for _, f := range []struct {
		name      string
		want, got bool
	}{
		{"Handicap", want.Handicap, got.Handicap},
		{"Lab", want.Lab, got.Lab},
		{"PlacesWithSocket", want.PlacesWithSocket, got.PlacesWithSocket},
		{"Exahm", want.Exahm, got.Exahm},
		{"Seb", want.Seb, got.Seb},
		{"Deactivated", want.Deactivated, got.Deactivated},
	} {
		if f.want != f.got {
			t.Errorf("%s = %v, want %v", f.name, f.got, f.want)
		}
	}
	if got.RequestWith != want.RequestWith {
		t.Errorf("RequestWith = %q, want %q", got.RequestWith, want.RequestWith)
	}
	if got.RequestPriority != want.RequestPriority {
		t.Errorf("RequestPriority = %d, want %d", got.RequestPriority, want.RequestPriority)
	}
	for _, f := range []struct {
		name      string
		want, got *int
	}{
		{"SebSeats", want.SebSeats, got.SebSeats},
		{"HmebSeats", want.HmebSeats, got.HmebSeats},
		{"Hitzewert", want.Hitzewert, got.Hitzewert},
	} {
		switch {
		case f.want == nil && f.got == nil:
		case f.want == nil || f.got == nil:
			t.Errorf("%s = %s, want %s", f.name, intval(f.got), intval(f.want))
		case *f.want != *f.got:
			t.Errorf("%s = %d, want %d", f.name, *f.got, *f.want)
		}
	}
}

func TestRoomRoundTrip(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	want := testRoom("R1.046")

	got, err := pg.AddRoom(ctx, want)
	if err != nil {
		t.Fatalf("AddRoom: %v", err)
	}
	assertRoomEqual(t, want, got)

	read, err := pg.RoomByName(ctx, "R1.046")
	if err != nil {
		t.Fatalf("RoomByName: %v", err)
	}
	assertRoomEqual(t, want, read)
}

func TestRoomNilSeatOverridesSurvive(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	want := testRoom("R1.046")
	want.SebSeats = nil
	want.HmebSeats = nil
	want.Hitzewert = nil

	if _, err := pg.AddRoom(ctx, want); err != nil {
		t.Fatalf("AddRoom: %v", err)
	}

	got, err := pg.RoomByName(ctx, "R1.046")
	if err != nil {
		t.Fatalf("RoomByName: %v", err)
	}
	assertRoomEqual(t, want, got)
}

// TestRoomNeedsRequestIsDerived is the test the real data asked for. In Mongo
// needsRequest was a stored field under two spellings, so it could disagree with
// requestWith; here the column is generated and a caller cannot write it wrong
// even by handing in a room struct that says otherwise.
func TestRoomNeedsRequestIsDerived(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	for _, tc := range []struct {
		name        string
		requestWith model.RoomRequestType
		claimed     bool
		want        bool
	}{
		{"R1.046", model.RoomRequestTypeManagement, false, true},
		{"R1.049", model.RoomRequestTypeAnny, false, true},
		{"R2.011", model.RoomRequestTypeNone, true, false},
	} {
		room := testRoom(tc.name)
		room.RequestWith = tc.requestWith
		room.NeedsRequest = tc.claimed

		got, err := pg.AddRoom(ctx, room)
		if err != nil {
			t.Fatalf("AddRoom(%s): %v", tc.name, err)
		}
		if got.NeedsRequest != tc.want {
			t.Errorf("%s: NeedsRequest = %v, want %v (requestWith %s, claimed %v)",
				tc.name, got.NeedsRequest, tc.want, tc.requestWith, tc.claimed)
		}

		read, err := pg.RoomByName(ctx, tc.name)
		if err != nil {
			t.Fatalf("RoomByName(%s): %v", tc.name, err)
		}
		if read.NeedsRequest != tc.want {
			t.Errorf("%s: re-read NeedsRequest = %v, want %v", tc.name, read.NeedsRequest, tc.want)
		}
	}
}

// TestRoomNeedsRequestFollowsAnUpdate pins the other half: flipping requestWith
// re-derives needsRequest without anybody having to remember to update it.
func TestRoomNeedsRequestFollowsAnUpdate(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	room := testRoom("R1.046")
	room.RequestWith = model.RoomRequestTypeManagement
	if _, err := pg.AddRoom(ctx, room); err != nil {
		t.Fatalf("AddRoom: %v", err)
	}

	room.RequestWith = model.RoomRequestTypeNone
	got, err := pg.ReplaceRoom(ctx, room)
	if err != nil {
		t.Fatalf("ReplaceRoom: %v", err)
	}
	if got.NeedsRequest {
		t.Error("NeedsRequest still true after requestWith was set to NONE")
	}
}

// TestRoomMissingIsAnError pins the difference to the NTA methods: a missing room
// is an error, not (nil, nil). plexams.UpdateRoom and BlockRoomForSlot rely on it.
func TestRoomMissingIsAnError(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	if _, err := pg.RoomByName(ctx, "R9.999"); err == nil {
		t.Error("RoomByName of a missing room returned no error")
	}
	if _, err := pg.ReplaceRoom(ctx, testRoom("R9.999")); err == nil {
		t.Error("ReplaceRoom of a missing room returned no error -- it must not upsert")
	}
	if _, err := pg.SetRoomDeactivated(ctx, "R9.999", true); err == nil {
		t.Error("SetRoomDeactivated of a missing room returned no error")
	}

	// ... and none of them created anything on the way.
	rooms, err := pg.Rooms(ctx)
	if err != nil {
		t.Fatalf("Rooms: %v", err)
	}
	if len(rooms) != 0 {
		t.Errorf("len(Rooms) = %d, want 0", len(rooms))
	}
}

func TestRoomHasRoomAndDuplicate(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	has, err := pg.HasRoom(ctx, "R1.046")
	if err != nil {
		t.Fatalf("HasRoom: %v", err)
	}
	if has {
		t.Error("HasRoom = true on an empty database")
	}

	if _, err := pg.AddRoom(ctx, testRoom("R1.046")); err != nil {
		t.Fatalf("AddRoom: %v", err)
	}

	has, err = pg.HasRoom(ctx, "R1.046")
	if err != nil {
		t.Fatalf("HasRoom: %v", err)
	}
	if !has {
		t.Error("HasRoom = false for a room that was just added")
	}

	// The Mongo collection had no unique index on name, so this used to produce a
	// second document. Here the primary key refuses it.
	if _, err := pg.AddRoom(ctx, testRoom("R1.046")); err == nil {
		t.Error("AddRoom accepted a duplicate name")
	}
}

func TestRoomSetDeactivated(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	if _, err := pg.AddRoom(ctx, testRoom("R1.046")); err != nil {
		t.Fatalf("AddRoom: %v", err)
	}

	got, err := pg.SetRoomDeactivated(ctx, "R1.046", true)
	if err != nil {
		t.Fatalf("SetRoomDeactivated: %v", err)
	}
	if !got.Deactivated {
		t.Error("Deactivated = false right after it was set to true")
	}

	// The flag must survive a full ReplaceRoom only if the caller carries it --
	// plexams.UpdateRoom does exactly that (the toggle owns the active state).
	read, err := pg.RoomByName(ctx, "R1.046")
	if err != nil {
		t.Fatalf("RoomByName: %v", err)
	}
	if !read.Deactivated {
		t.Error("Deactivated was not persisted")
	}
}

func TestRoomsEmptyIsNotNil(t *testing.T) {
	pg := pgtest.NewDB(t)

	rooms, err := pg.Rooms(t.Context())
	if err != nil {
		t.Fatalf("Rooms: %v", err)
	}
	if rooms == nil {
		t.Fatal("Rooms returned nil, want an empty slice")
	}
}

// TestRoomsSortedByteWise guards the collation pinned in the query. Mongo sorted
// room names by bytes; a de_DE collation weighs the dot differently and would
// reorder exactly the names that look most alike.
func TestRoomsSortedByteWise(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	for _, name := range []string{"R3.014", "R1.046", "R10.1", "R1.006", "T3.017"} {
		if _, err := pg.AddRoom(ctx, testRoom(name)); err != nil {
			t.Fatalf("AddRoom(%s): %v", name, err)
		}
	}

	rooms, err := pg.Rooms(ctx)
	if err != nil {
		t.Fatalf("Rooms: %v", err)
	}

	want := []string{"R1.006", "R1.046", "R10.1", "R3.014", "T3.017"}
	if len(rooms) != len(want) {
		t.Fatalf("len(Rooms) = %d, want %d", len(rooms), len(want))
	}
	for i, w := range want {
		if rooms[i].Name != w {
			t.Errorf("Rooms[%d].Name = %q, want %q", i, rooms[i].Name, w)
		}
	}
}
