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

func TestComputeSlotTimeSpec(t *testing.T) {
	// slots at 08:30, 10:30, 12:30, 14:30, 16:30 (day earliest start = 08:30).
	slots := fiveSlots()
	earliest := parseDayMinutes(defaultSlotTimeWinterEarliest, "10:00") // 600 (10:00)
	latest := parseDayMinutes(defaultSlotTimeSummerLatest, "14:00")     // 840 (14:00)
	const windowWeight, gradientWeight = 20000.0, 2.0

	// OFF → empty spec.
	if spec := computeSlotTimeSpec(slotTimeOff, true, windowWeight, gradientWeight, earliest, latest, slots); spec.weight != 0 || spec.severity != nil || spec.forbidden != nil {
		t.Errorf("off must yield an empty spec, got %+v", spec)
	}

	// Winter HARD: only 08:30 is before 10:00 → forbidden[0]; winter has no gradient, so no
	// soft term (weight 0).
	winterHard := computeSlotTimeSpec(slotTimeWinter, true, windowWeight, gradientWeight, earliest, latest, slots)
	if !winterHard.hard || winterHard.mode != examplan.TimeWindowWinter {
		t.Errorf("winter HARD: hard/mode wrong, got %+v", winterHard)
	}
	wantForbidden := []bool{true, false, false, false, false}
	for i := range wantForbidden {
		if winterHard.forbidden[i] != wantForbidden[i] {
			t.Errorf("winter HARD forbidden[%d] = %v, want %v (%v)", i, winterHard.forbidden[i], wantForbidden[i], winterHard.forbidden)
		}
	}
	if winterHard.weight != 0 || winterHard.severity != nil {
		t.Errorf("winter HARD should carry no soft term, got weight=%v severity=%v", winterHard.weight, winterHard.severity)
	}

	// Summer HARD: 14:30 and 16:30 are after 14:00 → forbidden; the mild gradient (hours later
	// than 08:30) is the only soft term, at the gradient weight.
	summerHard := computeSlotTimeSpec(slotTimeSummer, true, windowWeight, gradientWeight, earliest, latest, slots)
	if summerHard.mode != examplan.TimeWindowSummer || summerHard.weight != gradientWeight {
		t.Errorf("summer HARD: mode/weight wrong, got mode=%v weight=%v", summerHard.mode, summerHard.weight)
	}
	wantForbidden = []bool{false, false, false, true, true}
	for i := range wantForbidden {
		if summerHard.forbidden[i] != wantForbidden[i] {
			t.Errorf("summer HARD forbidden[%d] = %v, want %v", i, summerHard.forbidden[i], wantForbidden[i])
		}
	}
	wantGrad := []float64{0, 2, 4, 6, 8} // (start-08:30)/60h
	for i := range wantGrad {
		if summerHard.severity[i] != wantGrad[i] {
			t.Errorf("summer HARD gradient severity[%d] = %v, want %v", i, summerHard.severity[i], wantGrad[i])
		}
	}

	// Summer SOFT: no domain restriction; the (strong) window overflow and the (mild) gradient
	// fold into one severity at the window weight. 14:30 is 0.5h past 14:00, 16:30 is 2.5h past.
	summerSoft := computeSlotTimeSpec(slotTimeSummer, false, windowWeight, gradientWeight, earliest, latest, slots)
	if summerSoft.hard || summerSoft.forbidden != nil || summerSoft.weight != windowWeight {
		t.Errorf("summer SOFT: hard/forbidden/weight wrong, got %+v", summerSoft)
	}
	ratio := gradientWeight / windowWeight
	wantSoft := []float64{0, ratio * 2, ratio * 4, 0.5 + ratio*6, 2.5 + ratio*8}
	for i := range wantSoft {
		if diff := summerSoft.severity[i] - wantSoft[i]; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("summer SOFT severity[%d] = %v, want %v", i, summerSoft.severity[i], wantSoft[i])
		}
	}

	// Winter SOFT: window overflow only (08:30 is 1.5h before 10:00), no gradient.
	winterSoft := computeSlotTimeSpec(slotTimeWinter, false, windowWeight, gradientWeight, earliest, latest, slots)
	if winterSoft.weight != windowWeight || winterSoft.severity[0] != 1.5 {
		t.Errorf("winter SOFT: expected window penalty 1.5 at 08:30 with window weight, got weight=%v sev0=%v", winterSoft.weight, winterSoft.severity[0])
	}
	for i := 1; i < len(slots); i++ {
		if winterSoft.severity[i] != 0 {
			t.Errorf("winter SOFT severity[%d] should be 0 (in window), got %v", i, winterSoft.severity[i])
		}
	}
}
