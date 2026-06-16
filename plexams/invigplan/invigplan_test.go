package invigplan

import (
	"math/rand"
	"testing"
	"time"
)

// start builds a start time on a fixed test date for the given hour:minute.
func start(hour, min int) time.Time {
	return time.Date(2026, time.July, 6, hour, min, 0, 0, time.UTC)
}

func newRand() *rand.Rand { return rand.New(rand.NewSource(1)) }

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

func TestTimeWindowHard(t *testing.T) {
	// slot 1 at 08:00: a normal room (90 min -> ends 09:30) and an NTA room that
	// runs long (99 min -> ends 09:39). The reserve (Block 99) covers the longest
	// exam and therefore also ends 09:39.
	pos := []Position{
		{Day: 1, Slot: 1, Room: "R1", Minutes: 90, Block: 90, Start: start(8, 0)},
		{Day: 1, Slot: 1, Room: "R2", Minutes: 99, Block: 99, Start: start(8, 0), IsNTA: true},
		{Day: 1, Slot: 1, IsReserve: true, Minutes: 60, Block: 99, Start: start(8, 0)},
	}
	p := &Problem{
		Positions:    pos,
		Invigilators: []Invigilator{{ID: 1, TargetMinutes: 300}},
		Fixed:        map[int]int{},
		ToleranceMin: 60, MaxSpanHours: 8, Weights: DefaultWeights(),
	}
	// "until 09:30" on the test date: the normal room ends exactly 09:30 (allowed),
	// the NTA room and the reserve run to 09:39 (forbidden).
	p.Invigilators[0].TimeWindows = []DayTimeWindow{
		{Date: start(0, 0), Until: start(9, 30)},
	}
	p.Prepare()
	plan := NewPlan(p)

	c := timeWindowHard{}
	if !c.Allows(p, plan, 0, 1) {
		t.Error("normal room ending exactly at the until time must be allowed")
	}
	if c.Allows(p, plan, 1, 1) {
		t.Error("NTA room ending after the until time must be forbidden")
	}
	if c.Allows(p, plan, 2, 1) {
		t.Error("reserve covering the long NTA exam must be forbidden")
	}

	// from-bound: an invigilation starting before "from" is forbidden.
	p.Invigilators[0].TimeWindows = []DayTimeWindow{
		{Date: start(0, 0), From: start(8, 30)},
	}
	p.Prepare()
	if c.Allows(p, plan, 0, 1) {
		t.Error("start 08:00 before from 08:30 must be forbidden")
	}

	// a window on a different calendar date must not affect this date.
	p.Invigilators[0].TimeWindows = []DayTimeWindow{
		{Date: time.Date(2026, time.July, 7, 0, 0, 0, 0, time.UTC), Until: start(8, 30)},
	}
	p.Prepare()
	if !c.Allows(p, plan, 1, 1) {
		t.Error("window on another date must leave this date unrestricted")
	}

	// Check reports the violation for an assigned out-of-window position.
	p.Invigilators[0].TimeWindows = []DayTimeWindow{
		{Date: start(0, 0), Until: start(9, 30)},
	}
	p.Prepare()
	plan = NewPlan(p)
	plan.Set(1, 1) // assign the NTA room
	if vs := c.Check(p, plan); len(vs) != 1 {
		t.Fatalf("expected 1 time-window violation, got %d: %v", len(vs), vs)
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

// buildGridProblem builds a fully solvable problem: days × slotsPerDay slots,
// each with roomsPerSlot rooms plus a reserve, and enough invigilators to cover
// every slot one-per-person.
func buildGridProblem(days, slotsPerDay, roomsPerSlot, invigilators int) *Problem {
	starts := []time.Time{start(8, 0), start(10, 0), start(12, 0), start(14, 0)}
	var pos []Position
	totalMinutes := 0
	for d := 1; d <= days; d++ {
		for s := 1; s <= slotsPerDay; s++ {
			for r := 1; r <= roomsPerSlot; r++ {
				pos = append(pos, Position{
					Day: d, Slot: s, Room: roomName(r),
					Minutes: 90, Block: 90, Start: starts[s-1],
				})
				totalMinutes += 90
			}
			pos = append(pos, Position{
				Day: d, Slot: s, IsReserve: true,
				Minutes: 60, Block: 90, Start: starts[s-1],
			})
			totalMinutes += 60
		}
	}
	target := totalMinutes / invigilators
	invs := make([]Invigilator, 0, invigilators)
	for i := 1; i <= invigilators; i++ {
		invs = append(invs, Invigilator{ID: i, TargetMinutes: target})
	}
	p := &Problem{
		Positions: pos, Invigilators: invs, Fixed: map[int]int{},
		TimelagMin: 20, ToleranceMin: 90, MaxSpanHours: 8, Weights: DefaultWeights(),
	}
	p.Prepare()
	return p
}

func roomName(r int) string { return "R" + string(rune('0'+r)) }

func TestOptimizeIsFeasibleAndCovers(t *testing.T) {
	p := buildGridProblem(2, 2, 2, 6) // 12 positions, 6 invigilators
	reg := DefaultRegistry()
	opts := DefaultOptions()
	opts.Iterations = 50_000

	best, result := Optimize(p, reg, opts)

	if hv := reg.HardViolations(p, best); len(hv) != 0 {
		t.Fatalf("expected no hard violations, got %d: %v", len(hv), hv)
	}
	if result.Unfilled != 0 {
		t.Errorf("expected full coverage, %d positions unfilled", result.Unfilled)
	}
	if !result.BalanceSatisfied {
		t.Errorf("expected balance satisfied within tolerance")
	}
}

func TestGreedyRespectsFixed(t *testing.T) {
	p := buildGridProblem(1, 1, 2, 4)
	p.Fixed = map[int]int{0: 3} // first room fixed to invigilator 3
	p.Prepare()
	plan := Greedy(p, DefaultRegistry(), newRand())
	if plan.Assign[0] != 3 {
		t.Fatalf("greedy must keep fixed assignment, got %d", plan.Assign[0])
	}
}

func TestDaySpanIncludesOwnExams(t *testing.T) {
	// One invigilator with an own (multi-room) exam 08:00–09:30 they do NOT
	// invigilate, plus an assigned invigilation 16:30–18:00 on the same day.
	pos := []Position{
		{Day: 1, Slot: 1, Room: "R1", Minutes: 90, Block: 90, Start: start(16, 30)},
	}
	p := &Problem{
		Positions: pos,
		Invigilators: []Invigilator{{
			ID: 1, TargetMinutes: 90,
			OwnExams: []TimeSpan{{Day: 1, Start: start(8, 0), End: start(9, 30)}},
		}},
		Fixed: map[int]int{}, ToleranceMin: 60, MaxSpanHours: 8, Weights: DefaultWeights(),
	}
	p.Prepare()

	plan := NewPlan(p)
	plan.Set(0, 1)

	penalty, vs := daySpanSoft{}.Cost(p, plan)
	if penalty == 0 || len(vs) == 0 {
		t.Fatalf("expected a day-span penalty: 08:00 exam + 18:00 invigilation = 10h > 8h")
	}

	// Without the own exam the single 1.5h invigilation must not be penalized.
	p.Invigilators[0].OwnExams = nil
	alone, _ := daySpanSoft{}.Cost(p, plan)
	if alone != 0 {
		t.Errorf("expected no penalty for a 1.5h invigilation alone, got %g", alone)
	}
}

func TestMaxDaysCountsOwnExamDays(t *testing.T) {
	// invigilations on days 8, 12, 14; own exams on 8, 11, 12 (none excluded)
	// => present on 4 days {8,11,12,14} > 3 => penalty.
	pos := []Position{
		{Day: 8, Slot: 1, Room: "R1", Minutes: 90, Block: 90, Start: start(8, 0)},
		{Day: 12, Slot: 1, Room: "R1", Minutes: 90, Block: 90, Start: start(8, 0)},
		{Day: 14, Slot: 1, Room: "R1", Minutes: 90, Block: 90, Start: start(8, 0)},
	}
	p := &Problem{
		Positions: pos,
		Invigilators: []Invigilator{{
			ID: 1, TargetMinutes: 270,
			OwnExamDays:  map[int]bool{8: true, 11: true, 12: true},
			ExcludedDays: map[int]bool{},
		}},
		Fixed: map[int]int{}, ToleranceMin: 60, MaxSpanHours: 8, Weights: DefaultWeights(),
	}
	p.Prepare()
	plan := NewPlan(p)
	for i := range pos {
		plan.Set(i, 1)
	}

	penalty, vs := maxDaysSoft{}.Cost(p, plan)
	if penalty == 0 || len(vs) == 0 {
		t.Fatalf("expected penalty: present on 4 days (invig 8,12,14 + exam 11)")
	}

	// Excluding day 11 means the person is treated as not present there, so the
	// attendance is {8,12,14} = 3 days => no penalty.
	p.Invigilators[0].ExcludedDays = map[int]bool{11: true}
	alone, _ := maxDaysSoft{}.Cost(p, plan)
	if alone != 0 {
		t.Errorf("expected no penalty when exam day 11 is excluded, got %g", alone)
	}
}
