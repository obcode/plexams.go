package conflictcalc

import (
	"testing"
	"time"

	"github.com/obcode/plexams.go/graph/model"
)

func TestNormPair(t *testing.T) {
	if a, b := NormPair(7, 3); a != 3 || b != 7 {
		t.Errorf("NormPair(7,3) = (%d,%d), want (3,7)", a, b)
	}
	if a, b := NormPair(3, 7); a != 3 || b != 7 {
		t.Errorf("NormPair(3,7) = (%d,%d), want (3,7)", a, b)
	}
	if a, b := NormPair(5, 5); a != 5 || b != 5 {
		t.Errorf("NormPair(5,5) = (%d,%d), want (5,5)", a, b)
	}
}

func TestProximityRank(t *testing.T) {
	tests := []struct {
		label string
		want  int
	}{
		{SameSlot, 4},
		{Adjacent, 3},
		{SameDay, 2},
		{NextDay, 1},
		{"", 0},
		{"UNKNOWN", 0},
	}
	for _, tt := range tests {
		if got := ProximityRank(tt.label); got != tt.want {
			t.Errorf("ProximityRank(%q) = %d, want %d", tt.label, got, tt.want)
		}
	}
	// ranking must be strictly worst→mildest so worse/better comparisons hold
	ordered := []string{SameSlot, Adjacent, SameDay, NextDay, ""}
	for i := 1; i < len(ordered); i++ {
		if ProximityRank(ordered[i-1]) <= ProximityRank(ordered[i]) {
			t.Errorf("proximity ranks not strictly ordered at %q vs %q", ordered[i-1], ordered[i])
		}
	}
}

func slotAt(day, slot int, start time.Time) *model.Slot {
	return &model.Slot{DayNumber: day, SlotNumber: slot, Starttime: start}
}

func TestSlotProximity(t *testing.T) {
	day1 := time.Date(2026, 1, 12, 8, 30, 0, 0, time.Local)
	day2 := time.Date(2026, 1, 13, 8, 30, 0, 0, time.Local)
	day5 := time.Date(2026, 1, 16, 8, 30, 0, 0, time.Local)

	tests := []struct {
		name      string
		a, b      *model.Slot
		wantRank  int
		wantLabel string
	}{
		{"same slot", slotAt(1, 2, day1), slotAt(1, 2, day1), 4, SameSlot},
		{"adjacent slot", slotAt(1, 2, day1), slotAt(1, 3, day1), 3, Adjacent},
		{"adjacent slot reversed", slotAt(1, 3, day1), slotAt(1, 2, day1), 3, Adjacent},
		{"same day, two apart", slotAt(1, 2, day1), slotAt(1, 4, day1), 2, SameDay},
		{"next day", slotAt(1, 2, day1), slotAt(2, 2, day2), 1, NextDay},
		{"next day reversed", slotAt(2, 2, day2), slotAt(1, 2, day1), 1, NextDay},
		{"far apart", slotAt(1, 2, day1), slotAt(4, 2, day5), 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rank, label := SlotProximity(tt.a, tt.b)
			if rank != tt.wantRank || label != tt.wantLabel {
				t.Errorf("SlotProximity = (%d, %q), want (%d, %q)", rank, label, tt.wantRank, tt.wantLabel)
			}
		})
	}
}

func conflict(a1, a2 int, prox string) *model.ExamScheduleConflict {
	return &model.ExamScheduleConflict{Ancode1: a1, Ancode2: a2, Proximity: prox}
}

func TestDiffAgainstSaved(t *testing.T) {
	saved := []*model.ExamScheduleConflict{
		conflict(1, 2, Adjacent), // will stay ADJACENT -> unchanged
		conflict(1, 3, SameDay),  // will become ADJACENT -> worse
		conflict(1, 4, SameSlot), // will become SameDay -> better
		conflict(1, 5, Adjacent), // gone in generated -> resolved
	}
	generated := []*model.ExamScheduleConflict{
		conflict(1, 2, Adjacent),
		conflict(1, 3, Adjacent),
		conflict(1, 4, SameDay),
		conflict(1, 6, SameSlot), // not in saved -> new
	}

	resolved := DiffAgainstSaved(generated, saved)

	wantGen := map[[2]int]string{
		{1, 2}: "unchanged",
		{1, 3}: "worse",
		{1, 4}: "better",
		{1, 6}: "new",
	}
	for _, c := range generated {
		key := [2]int{c.Ancode1, c.Ancode2}
		if c.DiffStatus != wantGen[key] {
			t.Errorf("generated %v DiffStatus = %q, want %q", key, c.DiffStatus, wantGen[key])
		}
	}

	if len(resolved) != 1 {
		t.Fatalf("resolved count = %d, want 1", len(resolved))
	}
	if resolved[0].Ancode1 != 1 || resolved[0].Ancode2 != 5 {
		t.Errorf("resolved pair = (%d,%d), want (1,5)", resolved[0].Ancode1, resolved[0].Ancode2)
	}
	if resolved[0].DiffStatus != "resolved" {
		t.Errorf("resolved DiffStatus = %q, want %q", resolved[0].DiffStatus, "resolved")
	}
}

func TestDiffAgainstSavedEmpty(t *testing.T) {
	// no saved plan: everything is new, nothing resolved
	generated := []*model.ExamScheduleConflict{conflict(1, 2, SameSlot)}
	resolved := DiffAgainstSaved(generated, nil)
	if len(resolved) != 0 {
		t.Errorf("resolved = %d, want 0", len(resolved))
	}
	if generated[0].DiffStatus != "new" {
		t.Errorf("DiffStatus = %q, want new", generated[0].DiffStatus)
	}
}
