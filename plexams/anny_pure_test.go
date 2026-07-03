package plexams

import (
	"testing"
	"time"

	"github.com/obcode/plexams.go/graph/model"
)

// These are characterization tests: they pin down the CURRENT behaviour of the shared
// Anny/room booking helpers before the plexams package is decomposed, so a later refactor
// that moves this logic into its own package cannot change it unnoticed.

func TestSlotBlockDuration(t *testing.T) {
	tests := []struct {
		name       string
		starttimes []*model.Starttime
		want       time.Duration
	}{
		{"nil defaults to 120min", nil, 120 * time.Minute},
		{"single starttime defaults to 120min", []*model.Starttime{{Start: "08:30"}}, 120 * time.Minute},
		{"two starttimes give their difference", []*model.Starttime{{Start: "08:30"}, {Start: "10:30"}}, 120 * time.Minute},
		{"90 minute spacing", []*model.Starttime{{Start: "08:00"}, {Start: "09:30"}}, 90 * time.Minute},
		{"only first two matter", []*model.Starttime{{Start: "08:00"}, {Start: "10:00"}, {Start: "14:00"}}, 120 * time.Minute},
		{"unparseable falls back to 120min", []*model.Starttime{{Start: "foo"}, {Start: "bar"}}, 120 * time.Minute},
		{"non-increasing falls back to 120min", []*model.Starttime{{Start: "10:00"}, {Start: "08:00"}}, 120 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := slotBlockDuration(tt.starttimes); got != tt.want {
				t.Errorf("slotBlockDuration() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormRoomName(t *testing.T) {
	// normRoomName (preplan_booking.go) normalizes room names for the preplan room views.
	// It must stay identical to anny.normalizeRoomName (tested in the anny package).
	tests := []struct{ in, want string }{
		{"R1.006", "R1.006"},
		{" r1.006 ", "R1.006"},
		{"T 3.014", "T3.014"},
		{"  a b  c ", "ABC"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := normRoomName(tt.in); got != tt.want {
			t.Errorf("normRoomName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestMergeBookedEntriesByRoom(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 1, 20, h, m, 0, 0, time.Local) }

	t.Run("fewer than two entries returned unchanged", func(t *testing.T) {
		in := []BookedEntry{{From: at(8, 0), Until: at(10, 0), Rooms: []string{"R1.006"}, Approved: true}}
		got := mergeBookedEntriesByRoom(in)
		if len(got) != 1 {
			t.Fatalf("got %d entries, want 1", len(got))
		}
	})

	t.Run("adjacent same-room same-approval bookings merge", func(t *testing.T) {
		in := []BookedEntry{
			{From: at(8, 0), Until: at(10, 0), Rooms: []string{"R1.006"}, Approved: true},
			{From: at(10, 0), Until: at(12, 0), Rooms: []string{"R1.006"}, Approved: true},
		}
		got := mergeBookedEntriesByRoom(in)
		if len(got) != 1 {
			t.Fatalf("got %d entries, want 1 merged", len(got))
		}
		if !got[0].From.Equal(at(8, 0)) || !got[0].Until.Equal(at(12, 0)) {
			t.Errorf("merged span = [%v,%v], want [08:00,12:00]", got[0].From, got[0].Until)
		}
	})

	t.Run("overlapping same-room bookings merge to the later end", func(t *testing.T) {
		in := []BookedEntry{
			{From: at(8, 0), Until: at(11, 0), Rooms: []string{"R1.006"}, Approved: true},
			{From: at(10, 0), Until: at(12, 0), Rooms: []string{"R1.006"}, Approved: true},
		}
		got := mergeBookedEntriesByRoom(in)
		if len(got) != 1 || !got[0].Until.Equal(at(12, 0)) {
			t.Fatalf("got %+v, want single entry until 12:00", got)
		}
	})

	t.Run("different approval status does not merge", func(t *testing.T) {
		in := []BookedEntry{
			{From: at(8, 0), Until: at(10, 0), Rooms: []string{"R1.006"}, Approved: true},
			{From: at(10, 0), Until: at(12, 0), Rooms: []string{"R1.006"}, Approved: false},
		}
		got := mergeBookedEntriesByRoom(in)
		if len(got) != 2 {
			t.Errorf("got %d entries, want 2 (different approval)", len(got))
		}
	})

	t.Run("different rooms do not merge", func(t *testing.T) {
		in := []BookedEntry{
			{From: at(8, 0), Until: at(10, 0), Rooms: []string{"R1.006"}, Approved: true},
			{From: at(8, 0), Until: at(10, 0), Rooms: []string{"R1.049"}, Approved: true},
		}
		got := mergeBookedEntriesByRoom(in)
		if len(got) != 2 {
			t.Errorf("got %d entries, want 2 (different rooms)", len(got))
		}
	})

	t.Run("non-adjacent same-room bookings stay separate", func(t *testing.T) {
		in := []BookedEntry{
			{From: at(8, 0), Until: at(10, 0), Rooms: []string{"R1.006"}, Approved: true},
			{From: at(13, 0), Until: at(15, 0), Rooms: []string{"R1.006"}, Approved: true},
		}
		got := mergeBookedEntriesByRoom(in)
		if len(got) != 2 {
			t.Errorf("got %d entries, want 2 (gap between them)", len(got))
		}
	})
}
