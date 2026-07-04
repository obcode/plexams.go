package email

import (
	"testing"
	"time"

	"github.com/obcode/plexams.go/graph/model"
)

func at(hour, min int) time.Time { return time.Date(2026, 7, 6, hour, min, 0, 0, time.UTC) } // a Monday

func TestMergeRoomIntervals(t *testing.T) {
	cases := []struct {
		name string
		in   []roomInterval
		want []roomInterval
	}{
		{"empty", nil, nil},
		{"touching merge", []roomInterval{{at(8, 0), at(10, 0)}, {at(10, 0), at(12, 0)}},
			[]roomInterval{{at(8, 0), at(12, 0)}}},
		{"contained overlap", []roomInterval{{at(8, 0), at(10, 0)}, {at(9, 0), at(9, 30)}},
			[]roomInterval{{at(8, 0), at(10, 0)}}},
		{"disjoint kept", []roomInterval{{at(8, 0), at(9, 0)}, {at(10, 0), at(11, 0)}},
			[]roomInterval{{at(8, 0), at(9, 0)}, {at(10, 0), at(11, 0)}}},
		{"unsorted input", []roomInterval{{at(10, 0), at(12, 0)}, {at(8, 0), at(10, 0)}},
			[]roomInterval{{at(8, 0), at(12, 0)}}},
	}
	for _, c := range cases {
		got := mergeRoomIntervals(c.in)
		if len(got) != len(c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
			continue
		}
		for i := range got {
			if !got[i].start.Equal(c.want[i].start) || !got[i].end.Equal(c.want[i].end) {
				t.Errorf("%s[%d]: got %v-%v, want %v-%v", c.name, i, got[i].start, got[i].end, c.want[i].start, c.want[i].end)
			}
		}
	}
}

func req(room string, active bool, from, until time.Time) *model.RoomRequest {
	return &model.RoomRequest{Room: room, Active: active, From: from, Until: until}
}

func TestBuildRoomRequestRooms(t *testing.T) {
	requests := []*model.RoomRequest{
		req("B", true, at(10, 15), at(12, 15)),
		req("A", true, at(10, 15), at(12, 15)),
		req("A", true, at(8, 15), at(10, 15)),  // earlier -> sorts first within A
		req("A", false, at(8, 15), at(10, 15)), // inactive -> excluded
	}
	rooms := BuildRoomRequestRooms(requests)
	if len(rooms) != 2 || rooms[0].Room != "A" || rooms[1].Room != "B" {
		t.Fatalf("rooms = %+v (want A, B)", rooms)
	}
	if len(rooms[0].Days) != 1 || rooms[0].Days[0].Date != "Mo, 06.07.2026" {
		t.Fatalf("A days = %+v", rooms[0].Days)
	}
	times := rooms[0].Days[0].Times
	if len(times) != 2 || times[0].From != "08:15" || times[0].Until != "10:15" || times[1].From != "10:15" {
		t.Errorf("A times = %+v", times)
	}
}

func TestBuildSecretariatRooms(t *testing.T) {
	roomInfo := map[string]*model.Room{
		"R1": {Name: "R1"},
		"R2": {Name: "R2", NeedsRequest: true},
		"R3": {Name: "R3", Deactivated: true},
	}
	slotTime := func(_, _ int) time.Time { return at(8, 0) }
	plannedRooms := []*model.PlannedRoom{
		{RoomName: "R1", Day: 1, Slot: 1, Duration: 90},     // 08:00-09:30
		{RoomName: "R1", Day: 1, Slot: 1, Duration: 120},    // 08:00-10:00 -> merges to 08:00-10:00
		{RoomName: "R2", Day: 1, Slot: 1, Duration: 90},     // NeedsRequest -> skipped
		{RoomName: "R3", Day: 1, Slot: 1, Duration: 90},     // Deactivated -> skipped
		{RoomName: "ONLINE", Day: 1, Slot: 1, Duration: 90}, // pseudo-room -> skipped
		{RoomName: "RX", Day: 1, Slot: 1, Duration: 90},     // unknown -> skipped
	}
	rooms := BuildSecretariatRooms(plannedRooms, roomInfo, slotTime)
	if len(rooms) != 1 || rooms[0].Room != "R1" {
		t.Fatalf("rooms = %+v (want only R1)", rooms)
	}
	if len(rooms[0].Days) != 1 || len(rooms[0].Days[0].Times) != 1 {
		t.Fatalf("R1 days = %+v", rooms[0].Days)
	}
	tm := rooms[0].Days[0].Times[0]
	if tm.From != "08:00" || tm.Until != "10:00" {
		t.Errorf("R1 merged time = %+v, want 08:00-10:00", tm)
	}
}
