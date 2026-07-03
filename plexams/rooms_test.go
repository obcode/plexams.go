package plexams

import (
	"testing"
	"time"
)

func TestMergeBookedEntriesByRoomMergesAdjacentEntries(t *testing.T) {
	entries := []AnnyRoomBooking{
		{
			From:     time.Date(2026, 7, 22, 10, 30, 0, 0, time.Local),
			Until:    time.Date(2026, 7, 22, 14, 0, 0, 0, time.Local),
			Rooms:    []string{"T3.016"},
			Approved: true,
		},
		{
			From:     time.Date(2026, 7, 22, 14, 0, 0, 0, time.Local),
			Until:    time.Date(2026, 7, 22, 18, 30, 0, 0, time.Local),
			Rooms:    []string{"T3.016"},
			Approved: true,
		},
	}

	merged := mergeAnnyRoomBookings(entries)

	if len(merged) != 1 {
		t.Fatalf("expected 1 merged booking, got %d", len(merged))
	}

	if !merged[0].From.Equal(entries[0].From) {
		t.Fatalf("unexpected merged start: want %s, got %s", entries[0].From.Format(time.RFC3339), merged[0].From.Format(time.RFC3339))
	}
	if !merged[0].Until.Equal(entries[1].Until) {
		t.Fatalf("unexpected merged end: want %s, got %s", entries[1].Until.Format(time.RFC3339), merged[0].Until.Format(time.RFC3339))
	}
}

func TestMergeBookedEntriesByRoomKeepsDifferentApprovalSeparate(t *testing.T) {
	entries := []AnnyRoomBooking{
		{
			From:     time.Date(2026, 7, 22, 10, 30, 0, 0, time.Local),
			Until:    time.Date(2026, 7, 22, 14, 0, 0, 0, time.Local),
			Rooms:    []string{"T3.016"},
			Approved: true,
		},
		{
			From:     time.Date(2026, 7, 22, 14, 0, 0, 0, time.Local),
			Until:    time.Date(2026, 7, 22, 18, 30, 0, 0, time.Local),
			Rooms:    []string{"T3.016"},
			Approved: false,
		},
	}

	merged := mergeAnnyRoomBookings(entries)

	if len(merged) != 2 {
		t.Fatalf("expected 2 bookings when approval differs, got %d", len(merged))
	}
}
