package plexams

import (
	"testing"
	"time"

	"github.com/obcode/plexams.go/graph/model"
)

// sday builds a local time on 2026-07-16.
func sday(hh, mm int) time.Time {
	return time.Date(2026, 7, 16, hh, mm, 0, 0, time.Local)
}

func TestSubtractWindows(t *testing.T) {
	day := timeWindow{sday(0, 0), sday(0, 0).AddDate(0, 0, 1)}

	t.Run("nothing blocked keeps the whole day", func(t *testing.T) {
		free := subtractWindows(day, nil)
		if len(free) != 1 || !free[0].from.Equal(day.from) || !free[0].until.Equal(day.until) {
			t.Errorf("expected the full day, got %v", free)
		}
	})

	t.Run("two foreign bookings cut two holes", func(t *testing.T) {
		free := subtractWindows(day, []timeWindow{
			{sday(8, 0), sday(11, 0)},
			{sday(14, 0), sday(15, 30)},
		})
		want := []timeWindow{
			{day.from, sday(8, 0)},
			{sday(11, 0), sday(14, 0)},
			{sday(15, 30), day.until},
		}
		if len(free) != len(want) {
			t.Fatalf("expected %d free windows, got %v", len(want), free)
		}
		for i, w := range want {
			if !free[i].from.Equal(w.from) || !free[i].until.Equal(w.until) {
				t.Errorf("window %d = %v–%v, want %v–%v", i, free[i].from, free[i].until, w.from, w.until)
			}
		}
	})

	t.Run("overlapping bookings merge into one hole", func(t *testing.T) {
		free := subtractWindows(day, []timeWindow{
			{sday(8, 0), sday(11, 0)},
			{sday(10, 0), sday(13, 0)},
		})
		if len(free) != 2 || !free[1].from.Equal(sday(13, 0)) {
			t.Errorf("expected the free part to resume at 13:00, got %v", free)
		}
	})

	t.Run("a booking over the whole day leaves nothing", func(t *testing.T) {
		if free := subtractWindows(day, []timeWindow{day}); len(free) != 0 {
			t.Errorf("expected no free window, got %v", free)
		}
	})
}

func TestSlotSeatsFromIntervals(t *testing.T) {
	block := 120 * time.Minute
	intervals := []bookedRoomInterval{
		{room: "T3.015", from: sday(8, 0), until: sday(12, 0), exahm: true, seats: 30, sebSeats: 30},
		// too short for the 10:30 block (ends 12:00 < 12:30)
		{room: "T3.016", from: sday(8, 0), until: sday(11, 0), exahm: true, seats: 30, sebSeats: 30},
	}
	if got := slotSeatsFromIntervals(intervals, sday(8, 30), block); got != 60 {
		t.Errorf("08:30 slot = %d seats, want 60", got)
	}
	if got := slotSeatsFromIntervals(intervals, sday(10, 30), block); got != 0 {
		t.Errorf("10:30 slot = %d seats, want 0 (no room covers until 12:30)", got)
	}
}

// TestRoomsToBookForSlot: an EXaHM exam for 50 students in a slot where nothing is booked
// yet must be covered by the two largest free EXaHM rooms, for the exam window incl. the
// 30/30 buffers — and rooms we already hold must not be proposed again.
func TestRoomsToBookForSlot(t *testing.T) {
	block := 120 * time.Minute
	start := sday(10, 30)
	exam := &model.PreplanExam{
		ID: 1, Module: "Softwareentwicklung I", ExamKind: "EXaHM",
		ExpectedStudents: 50, Duration: iptr(90),
	}
	free := []bookedRoomInterval{
		{room: "T3.015", from: sday(0, 0), until: sday(20, 0), exahm: true, seb: true, seats: 30, sebSeats: 30},
		{room: "T3.016", from: sday(0, 0), until: sday(20, 0), exahm: true, seb: true, seats: 30, sebSeats: 30},
		{room: "T3.021", from: sday(0, 0), until: sday(20, 0), exahm: true, seb: true, seats: 1, sebSeats: 1},
	}

	t.Run("nothing booked yet", func(t *testing.T) {
		got := roomsToBookForSlot(start, []*model.PreplanExam{exam}, free, nil, block, 0)
		if len(got) != 2 {
			t.Fatalf("expected 2 rooms to book, got %d: %v", len(got), got)
		}
		for _, s := range got {
			if s.Room == "T3.021" {
				t.Errorf("the 1-seat NTA room must not be picked while big rooms are free")
			}
			// window = 10:00–12:30 (90 min exam + 30/30 buffer)
			if !s.From.Equal(sday(10, 0)) || !s.Until.Equal(sday(12, 30)) {
				t.Errorf("%s window = %v–%v, want 10:00–12:30", s.Room, s.From, s.Until)
			}
		}
	})

	t.Run("one room already ours", func(t *testing.T) {
		own := []bookedRoomInterval{
			{room: "T3.015", from: sday(9, 0), until: sday(13, 0), exahm: true, seats: 30, sebSeats: 30},
		}
		got := roomsToBookForSlot(start, []*model.PreplanExam{exam}, free, own, block, 0)
		if len(got) != 1 || got[0].Room != "T3.016" {
			t.Fatalf("expected only T3.016 to be proposed, got %v", got)
		}
	})

	t.Run("demand already covered by our bookings", func(t *testing.T) {
		own := []bookedRoomInterval{
			{room: "T3.015", from: sday(9, 0), until: sday(13, 0), exahm: true, seats: 30, sebSeats: 30},
			{room: "T3.016", from: sday(9, 0), until: sday(13, 0), exahm: true, seats: 30, sebSeats: 30},
		}
		if got := roomsToBookForSlot(start, []*model.PreplanExam{exam}, free, own, block, 0); len(got) != 0 {
			t.Errorf("expected no proposal, got %v", got)
		}
	})

	t.Run("SEB may not consume an EXaHM-only demand", func(t *testing.T) {
		sebOnly := []bookedRoomInterval{
			{room: "R-Lab", from: sday(0, 0), until: sday(20, 0), seb: true, seats: 60, sebSeats: 60},
			{room: "T3.015", from: sday(0, 0), until: sday(20, 0), exahm: true, seb: true, seats: 30, sebSeats: 30},
			{room: "T3.016", from: sday(0, 0), until: sday(20, 0), exahm: true, seb: true, seats: 30, sebSeats: 30},
		}
		got := roomsToBookForSlot(start, []*model.PreplanExam{exam}, sebOnly, nil, block, 0)
		for _, s := range got {
			if s.Room == "R-Lab" {
				t.Errorf("an EXaHM exam must not be seated in a SEB-only room: %v", got)
			}
		}
	})
}

func TestMergeBookingSuggestions(t *testing.T) {
	t0, t1, t2 := sday(8, 0), sday(10, 0), sday(12, 30)
	in := []*model.PreplanBookingSuggestion{
		{Room: "T3.015", From: sday(8, 0), Until: sday(10, 30), Seats: 30, Starttimes: []*time.Time{&t0}, Modules: []string{"A"}, Kinds: []string{"EXaHM"}},
		// abuts the previous one → one booking 08:00–12:30
		{Room: "T3.015", From: sday(10, 0), Until: sday(12, 30), Seats: 30, Starttimes: []*time.Time{&t1}, Modules: []string{"B"}, Kinds: []string{"SEB"}},
		// separate window later that day
		{Room: "T3.015", From: sday(14, 0), Until: sday(16, 30), Seats: 30, Starttimes: []*time.Time{&t2}, Modules: []string{"C"}, Kinds: []string{"SEB"}},
		{Room: "T3.016", From: sday(8, 0), Until: sday(10, 30), Seats: 30, Starttimes: []*time.Time{&t0}, Modules: []string{"A"}, Kinds: []string{"EXaHM"}},
	}
	got := mergeBookingSuggestions(in)
	if len(got) != 3 {
		t.Fatalf("expected 3 merged bookings, got %d: %v", len(got), got)
	}
	if got[0].Room != "T3.015" || !got[0].Until.Equal(sday(12, 30)) {
		t.Errorf("first booking = %v %v–%v, want T3.015 08:00–12:30", got[0].Room, got[0].From, got[0].Until)
	}
	if len(got[0].Modules) != 2 || len(got[0].Kinds) != 2 || len(got[0].Starttimes) != 2 {
		t.Errorf("merged booking should carry both modules/kinds/slots, got %v", got[0])
	}
	if got[1].Room != "T3.016" {
		t.Errorf("bookings must be sorted by time then room, got %v", got[1].Room)
	}
}
