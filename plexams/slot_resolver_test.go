package plexams

import (
	"testing"
	"time"

	"github.com/obcode/plexams.go/graph/model"
)

// TestSlotForTimeRoundTrip verifies the time↔slot conversion used to derive a plan
// entry's day/slot from its persisted Starttime (and back).
func TestSlotForTimeRoundTrip(t *testing.T) {
	p := &Plexams{} // no dbClient: deriveSemesterConfig guards the resolver wiring
	p.deriveSemesterConfig(&model.SemesterConfigInput{
		From:       time.Date(2026, 7, 6, 0, 0, 0, 0, time.Local),
		Until:      time.Date(2026, 7, 10, 0, 0, 0, 0, time.Local),
		StartTimes: []string{"08:30", "10:30"},
	})

	// Exact slot start round-trips to its (day, slot).
	start11, ok := p.TimeForSlot(1, 1)
	if !ok {
		t.Fatal("TimeForSlot(1,1): no such slot")
	}
	if d, s := p.SlotForTime(start11); d != 1 || s != 1 {
		t.Errorf("SlotForTime(%v) = (%d,%d), want (1,1)", start11, d, s)
	}

	// A time inside the slot's window (off-grid) still maps to that slot.
	if d, s := p.SlotForTime(start11.Add(20 * time.Minute)); d != 1 || s != 1 {
		t.Errorf("off-grid SlotForTime = (%d,%d), want (1,1)", d, s)
	}

	// The second start time maps to slot 2.
	start12, ok := p.TimeForSlot(1, 2)
	if !ok {
		t.Fatal("TimeForSlot(1,2): no such slot")
	}
	if d, s := p.SlotForTime(start12); d != 1 || s != 2 {
		t.Errorf("SlotForTime(%v) = (%d,%d), want (1,2)", start12, d, s)
	}

	// A time far outside the exam period matches no slot.
	if d, s := p.SlotForTime(start11.AddDate(0, 0, 60)); d != 0 || s != 0 {
		t.Errorf("outside-period SlotForTime = (%d,%d), want (0,0)", d, s)
	}
}
