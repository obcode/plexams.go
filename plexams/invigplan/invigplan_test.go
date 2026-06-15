package invigplan

import (
	"testing"
	"time"
)

// start builds a start time on a fixed test date for the given hour:minute.
func start(hour, min int) time.Time {
	return time.Date(2026, time.July, 6, hour, min, 0, 0, time.UTC)
}

// newTestProblem builds a tiny but realistic problem:
//
//	day 1, slot 1 (08:00) and slot 2 (09:45), each with rooms R1, R2 and a reserve.
//	two invigilators (1, 2) with no restrictions and a 300 min target.
func newTestProblem() *Problem {
	pos := []Position{
		{Day: 1, Slot: 1, Room: "R1", Minutes: 90, Block: 90, Start: start(8, 0)},
		{Day: 1, Slot: 1, Room: "R2", Minutes: 90, Block: 90, Start: start(8, 0), IsNTA: true},
		{Day: 1, Slot: 1, IsReserve: true, Minutes: 60, Block: 90, Start: start(8, 0)},
		{Day: 1, Slot: 2, Room: "R1", Minutes: 90, Block: 90, Start: start(9, 45)},
		{Day: 1, Slot: 2, IsReserve: true, Minutes: 60, Block: 90, Start: start(9, 45)},
	}
	p := &Problem{
		Positions: pos,
		Invigilators: []Invigilator{
			{ID: 1, TargetMinutes: 300},
			{ID: 2, TargetMinutes: 300},
		},
		Fixed:        map[int]int{},
		TimelagMin:   20,
		ToleranceMin: 60,
		MaxSpanHours: 8,
		Weights:      DefaultWeights(),
	}
	p.Prepare()
	return p
}

func TestPlanSetClearKeepsInverseIndex(t *testing.T) {
	p := newTestProblem()
	plan := NewPlan(p)

	plan.Set(0, 1)
	plan.Set(3, 1)
	if got := plan.Positions(1); len(got) != 2 || got[0] != 0 || got[1] != 3 {
		t.Fatalf("expected sorted [0 3], got %v", got)
	}
	if got := plan.DoingMinutes(1); got != 180 {
		t.Fatalf("expected 180 doing minutes, got %d", got)
	}
	plan.Clear(0)
	if got := plan.Positions(1); len(got) != 1 || got[0] != 3 {
		t.Fatalf("expected [3] after clear, got %v", got)
	}
	if got := plan.DoingMinutes(1); got != 90 {
		t.Fatalf("expected 90 doing minutes after clear, got %d", got)
	}
}

func TestFixedPositionsAreImmutable(t *testing.T) {
	p := newTestProblem()
	p.Fixed = map[int]int{2: 1} // reserve in slot 1 pre-planned to invigilator 1
	plan := NewPlan(p)

	if plan.Assign[2] != 1 {
		t.Fatalf("fixed position not applied, got %d", plan.Assign[2])
	}
	plan.Set(2, 2) // must be ignored
	if plan.Assign[2] != 1 {
		t.Fatalf("fixed position was changed to %d", plan.Assign[2])
	}
	if DefaultRegistry().Allows(p, plan, 2, 2) {
		t.Fatalf("registry should not allow reassigning a fixed position")
	}
	if !DefaultRegistry().Allows(p, plan, 2, 1) {
		t.Fatalf("registry should allow the locked invigilator on a fixed position")
	}
}

func TestAvailabilityHard(t *testing.T) {
	p := newTestProblem()
	p.Invigilators[0].ExcludedDays = map[int]bool{1: true}
	p.Invigilators[1].OnlyInSlots = map[[2]int]bool{{1, 2}: true}
	p.Prepare()
	plan := NewPlan(p)

	c := availabilityHard{}
	if c.Allows(p, plan, 0, 1) {
		t.Error("excluded day should not be allowed")
	}
	if c.Allows(p, plan, 0, 2) {
		t.Error("onlyInSlots should forbid slot 1 for invigilator 2")
	}
	if !c.Allows(p, plan, 3, 2) {
		t.Error("onlyInSlots should allow slot 2 for invigilator 2")
	}
}

func TestOwnExamHard(t *testing.T) {
	p := newTestProblem()
	p.Invigilators[0].OwnExamSlots = map[[2]int]bool{{1, 1}: true}
	p.Prepare()
	plan := NewPlan(p)

	c := ownExamHard{}
	if c.Allows(p, plan, 0, 1) {
		t.Error("own exam in slot must forbid invigilation there")
	}
	if !c.Allows(p, plan, 3, 1) {
		t.Error("other slot must stay allowed")
	}
}

func TestOnePerSlotHard(t *testing.T) {
	p := newTestProblem()
	plan := NewPlan(p)
	plan.Set(0, 1) // R1 slot 1

	c := oneInvigilationPerSlotHard{}
	if c.Allows(p, plan, 1, 1) {
		t.Error("invigilator already in slot 1 must not take a second position there")
	}
	if !c.Allows(p, plan, 3, 1) {
		t.Error("invigilator should be allowed in a different slot")
	}
}

func TestTimeGapHard(t *testing.T) {
	p := newTestProblem()
	plan := NewPlan(p)
	plan.Set(0, 1) // slot 1, 08:00-09:30

	c := timeGapHard{}
	// slot 2 starts 09:45 -> only 15 min after end 09:30, less than 20 min lag.
	if c.Allows(p, plan, 3, 1) {
		t.Error("time gap of 15 min should be too small for 20 min lag")
	}
	p.TimelagMin = 10
	if !c.Allows(p, plan, 3, 1) {
		t.Error("time gap of 15 min should be fine for 10 min lag")
	}
}

func TestMinuteBalanceSoft(t *testing.T) {
	p := newTestProblem()
	plan := NewPlan(p)
	// invigilator 1 does 90 min -> 210 off target (300). > tolerance 60.
	plan.Set(0, 1)

	_, vs := minuteBalanceSoft{}.Cost(p, plan)
	found := false
	for _, v := range vs {
		if v.InvigilatorID == 1 {
			found = true
		}
	}
	if !found {
		t.Error("expected a balance violation for invigilator 1")
	}
	if p.BalanceSatisfied(plan) {
		t.Error("balance must not be satisfied when 210 min off target")
	}
}

func TestCoverageSoft(t *testing.T) {
	p := newTestProblem()
	plan := NewPlan(p)
	plan.Set(0, 1)

	penalty, vs := coverageSoft{}.Cost(p, plan)
	if len(vs) != 4 { // 5 positions, 1 filled
		t.Fatalf("expected 4 unfilled positions, got %d", len(vs))
	}
	if penalty != p.Weights.Coverage*4 {
		t.Fatalf("unexpected coverage penalty %g", penalty)
	}
}

func TestMaxDaysSoft(t *testing.T) {
	// Build a 4-day problem with one invigilator and one room per day.
	pos := make([]Position, 0, 4)
	for d := 1; d <= 4; d++ {
		pos = append(pos, Position{Day: d, Slot: 1, Room: "R1", Minutes: 90, Block: 90, Start: start(8, 0)})
	}
	p := &Problem{
		Positions:    pos,
		Invigilators: []Invigilator{{ID: 1, TargetMinutes: 360}},
		Fixed:        map[int]int{},
		ToleranceMin: 60,
		MaxSpanHours: 8,
		Weights:      DefaultWeights(),
	}
	p.Prepare()
	plan := NewPlan(p)
	for i := range pos {
		plan.Set(i, 1)
	}

	penalty, vs := maxDaysSoft{}.Cost(p, plan)
	if penalty == 0 || len(vs) == 0 {
		t.Error("expected a max-days penalty for 4 invigilation days")
	}
}

func TestDistributionSoftEqualizes(t *testing.T) {
	p := newTestProblem()

	// both reserves to invigilator 1 -> higher penalty than one each.
	clustered := NewPlan(p)
	clustered.Set(2, 1)
	clustered.Set(4, 1)
	clusterPenalty, _ := distributionSoft{kind: KindReserve}.Cost(p, clustered)

	spread := NewPlan(p)
	spread.Set(2, 1)
	spread.Set(4, 2)
	spreadPenalty, _ := distributionSoft{kind: KindReserve}.Cost(p, spread)

	if !(clusterPenalty > spreadPenalty) {
		t.Errorf("clustered reserves (%g) should cost more than spread (%g)", clusterPenalty, spreadPenalty)
	}
}

func TestRegistryAllowsCombinesHardConstraints(t *testing.T) {
	p := newTestProblem()
	p.Invigilators[0].ExcludedDays = map[int]bool{1: true}
	p.Prepare()
	plan := NewPlan(p)

	r := DefaultRegistry()
	if r.Allows(p, plan, 0, 1) {
		t.Error("registry must reject excluded-day assignment")
	}
	if !r.Allows(p, plan, 0, 2) {
		t.Error("registry must accept a feasible assignment")
	}
}
