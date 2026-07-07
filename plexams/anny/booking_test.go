package anny

import (
	"testing"
	"time"

	"github.com/obcode/plexams.go/graph/model"
)

func at(h, m int) time.Time { return time.Date(2026, 1, 20, h, m, 0, 0, time.Local) }

func TestMergeRoomBookings(t *testing.T) {
	t.Run("fewer than two entries returned unchanged", func(t *testing.T) {
		in := []RoomBooking{{From: at(8, 0), Until: at(10, 0), Rooms: []string{"R1.006"}, Approved: true}}
		got := MergeRoomBookings(in)
		if len(got) != 1 {
			t.Fatalf("got %d entries, want 1", len(got))
		}
	})

	t.Run("adjacent same-room same-approval bookings merge", func(t *testing.T) {
		in := []RoomBooking{
			{From: at(8, 0), Until: at(10, 0), Rooms: []string{"R1.006"}, Approved: true},
			{From: at(10, 0), Until: at(12, 0), Rooms: []string{"R1.006"}, Approved: true},
		}
		got := MergeRoomBookings(in)
		if len(got) != 1 {
			t.Fatalf("got %d entries, want 1 merged", len(got))
		}
		if !got[0].From.Equal(at(8, 0)) || !got[0].Until.Equal(at(12, 0)) {
			t.Errorf("merged span = [%v,%v], want [08:00,12:00]", got[0].From, got[0].Until)
		}
	})

	t.Run("overlapping same-room bookings merge to the later end", func(t *testing.T) {
		in := []RoomBooking{
			{From: at(8, 0), Until: at(11, 0), Rooms: []string{"R1.006"}, Approved: true},
			{From: at(10, 0), Until: at(12, 0), Rooms: []string{"R1.006"}, Approved: true},
		}
		got := MergeRoomBookings(in)
		if len(got) != 1 || !got[0].Until.Equal(at(12, 0)) {
			t.Fatalf("got %+v, want single entry until 12:00", got)
		}
	})

	t.Run("different approval status does not merge", func(t *testing.T) {
		in := []RoomBooking{
			{From: at(8, 0), Until: at(10, 0), Rooms: []string{"R1.006"}, Approved: true},
			{From: at(10, 0), Until: at(12, 0), Rooms: []string{"R1.006"}, Approved: false},
		}
		got := MergeRoomBookings(in)
		if len(got) != 2 {
			t.Errorf("got %d entries, want 2 (different approval)", len(got))
		}
	})

	t.Run("different rooms do not merge", func(t *testing.T) {
		in := []RoomBooking{
			{From: at(8, 0), Until: at(10, 0), Rooms: []string{"R1.006"}, Approved: true},
			{From: at(8, 0), Until: at(10, 0), Rooms: []string{"R1.049"}, Approved: true},
		}
		got := MergeRoomBookings(in)
		if len(got) != 2 {
			t.Errorf("got %d entries, want 2 (different rooms)", len(got))
		}
	})

	t.Run("non-adjacent same-room bookings stay separate", func(t *testing.T) {
		in := []RoomBooking{
			{From: at(8, 0), Until: at(10, 0), Rooms: []string{"R1.006"}, Approved: true},
			{From: at(13, 0), Until: at(15, 0), Rooms: []string{"R1.006"}, Approved: true},
		}
		got := MergeRoomBookings(in)
		if len(got) != 2 {
			t.Errorf("got %d entries, want 2 (gap between them)", len(got))
		}
	})

	t.Run("same room with different casing/whitespace normalizes and merges (unsorted input)", func(t *testing.T) {
		in := []RoomBooking{
			{From: at(10, 0), Until: at(12, 0), Rooms: []string{" r1.006 "}, Approved: true},
			{From: at(8, 0), Until: at(10, 0), Rooms: []string{"R1.006"}, Approved: true},
		}
		got := MergeRoomBookings(in)
		if len(got) != 1 {
			t.Errorf("got %d entries, want 1 (normalized same room, unsorted input)", len(got))
		}
	})
}

func TestRoomBookedDuringExamTime(t *testing.T) {
	slot := &model.Slot{Starttime: at(8, 0)} // exam window 08:00–09:30(+90=09:30)
	cases := []struct {
		name  string
		books []RoomBooking
		want  bool
	}{
		{"nil slot", nil, false},
		{"covers exactly (inclusive bounds)", []RoomBooking{{From: at(8, 0), Until: at(9, 30)}}, true},
		{"covers with margin", []RoomBooking{{From: at(7, 0), Until: at(11, 0)}}, true},
		{"starts too late", []RoomBooking{{From: at(8, 1), Until: at(11, 0)}}, false},
		{"ends too early", []RoomBooking{{From: at(7, 0), Until: at(9, 29)}}, false},
		{"one of many covers", []RoomBooking{{From: at(8, 1), Until: at(9, 0)}, {From: at(7, 0), Until: at(10, 0)}}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := slot
			if c.name == "nil slot" {
				s = nil
			}
			if got := RoomBookedDuringExamTime(c.books, s); got != c.want {
				t.Errorf("RoomBookedDuringExamTime = %v, want %v", got, c.want)
			}
		})
	}
}

func TestCovers(t *testing.T) {
	winStart, winEnd := at(8, 0), at(9, 45) // required window [08:00, 09:45]
	cases := []struct {
		name        string
		from, until time.Time
		want        bool
	}{
		{"covers strictly", at(7, 59), at(9, 46), true},
		{"covers exactly", at(8, 0), at(9, 45), true},
		{"starts too late", at(8, 1), at(9, 46), false},
		{"ends too early", at(7, 59), at(9, 44), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Covers(c.from, c.until, winStart, winEnd); got != c.want {
				t.Errorf("Covers = %v, want %v", got, c.want)
			}
		})
	}
}
