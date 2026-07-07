package plexams

import (
	"testing"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/examplan"
)

func TestIsSummerSemester(t *testing.T) {
	cases := map[string]bool{"2026 SS": true, "2025 WS": false, "2026-SS": true, "": false}
	for sem, want := range cases {
		p := &Plexams{semester: sem}
		if got := p.isSummerSemester(); got != want {
			t.Errorf("isSummerSemester(%q) = %v, want %v", sem, got, want)
		}
	}
}

func TestResolveSlotTimeMode(t *testing.T) {
	summer := &Plexams{semester: "2026 SS"}
	winter := &Plexams{semester: "2025 WS"}
	if summer.resolveSlotTimeMode(model.SlotTimeConstraintModeAuto) != slotTimeSummer {
		t.Error("AUTO in a summer semester should resolve to summer")
	}
	if winter.resolveSlotTimeMode(model.SlotTimeConstraintModeAuto) != slotTimeWinter {
		t.Error("AUTO in a winter semester should resolve to winter")
	}
	// explicit overrides ignore the semester
	if summer.resolveSlotTimeMode(model.SlotTimeConstraintModeWinter) != slotTimeWinter {
		t.Error("WINTER override should force winter even in a summer semester")
	}
	if winter.resolveSlotTimeMode(model.SlotTimeConstraintModeSummer) != slotTimeSummer {
		t.Error("SUMMER override should force summer even in a winter semester")
	}
	if winter.resolveSlotTimeMode(model.SlotTimeConstraintModeOff) != slotTimeOff {
		t.Error("OFF should resolve to off")
	}
}

func TestParseDayMinutes(t *testing.T) {
	if m := parseDayMinutes("10:00", "09:00"); m != 600 {
		t.Errorf("parseDayMinutes(10:00) = %d, want 600", m)
	}
	if m := parseDayMinutes("13:30", "09:00"); m != 810 {
		t.Errorf("parseDayMinutes(13:30) = %d, want 810", m)
	}
	if m := parseDayMinutes("nonsense", "13:00"); m != 780 {
		t.Errorf("parseDayMinutes(invalid) should fall back to default 13:00 (780), got %d", m)
	}
}

// fiveSlots builds one day with the standard five start times.
func fiveSlots() []examplan.Slot {
	starts := []struct{ h, m int }{{8, 30}, {10, 30}, {12, 30}, {14, 30}, {16, 30}}
	slots := make([]examplan.Slot, len(starts))
	for i, s := range starts {
		start := time.Date(2026, 2, 2, s.h, s.m, 0, 0, time.Local)
		slots[i] = examplan.Slot{SlotRef: examplan.SlotRef{Start: start}}
	}
	return slots
}

func TestComputeSlotTimeSeverity(t *testing.T) {
	slots := fiveSlots()
	earliest := parseDayMinutes(defaultSlotTimeWinterEarliest, "10:00") // 600

	// winter, phase B: threshold — only 08:30 is before 10:00 → severity (600-510)/60 = 1.5.
	sev, w := computeSlotTimeSeverity(slotTimeWinter, 5, earliest, slots, false)
	if w != 5 {
		t.Errorf("phase-B weight should be the full weight, got %v", w)
	}
	want := []float64{1.5, 0, 0, 0, 0}
	for i := range want {
		if sev[i] != want[i] {
			t.Errorf("winter severity[%d] = %v, want %v (%v)", i, sev[i], want[i], sev)
		}
	}

	// summer, phase B: monotonic — hours later than the earliest start (08:30). The later,
	// the worse, so earlier is always strictly better (and, weighted by seats, large exams
	// go first).
	sev, _ = computeSlotTimeSeverity(slotTimeSummer, 5, earliest, slots, false)
	want = []float64{0, 2, 4, 6, 8}
	for i := range want {
		if sev[i] != want[i] {
			t.Errorf("summer severity[%d] = %v, want %v (%v)", i, sev[i], want[i], sev)
		}
	}

	// phase A (T-Bau) in summer / off → no penalty at all (go by the booking).
	if sev, w := computeSlotTimeSeverity(slotTimeSummer, 5, earliest, slots, true); sev != nil || w != 0 {
		t.Errorf("phase-A summer must disable the penalty, got sev=%v w=%v", sev, w)
	}
	if _, w := computeSlotTimeSeverity(slotTimeOff, 5, earliest, slots, false); w != 0 {
		t.Errorf("off must disable the penalty, got w=%v", w)
	}

	// phase A (T-Bau) in winter → gentle pull only: reduced weight, but 08:30 still penalized.
	sev, w = computeSlotTimeSeverity(slotTimeWinter, 5, earliest, slots, true)
	if w != 5*tbauSlotTimePullFactor {
		t.Errorf("phase-A winter weight should be reduced to %v, got %v", 5*tbauSlotTimePullFactor, w)
	}
	if sev[0] != 1.5 {
		t.Errorf("phase-A winter should still prefer later T-Bau starts (08:30 severity 1.5), got %v", sev[0])
	}
}
