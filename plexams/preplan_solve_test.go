package plexams

import "testing"

func progSet(ps ...string) map[string]bool {
	m := make(map[string]bool, len(ps))
	for _, p := range ps {
		m[p] = true
	}
	return m
}

func unit(id, seats int, exahm bool, progs ...string) *preplanUnit {
	drop := preplanDropBase + seats
	if exahm {
		drop += preplanExahmKeep
	}
	return &preplanUnit{members: []int{id}, seats: seats, programs: progSet(progs...), hasExahm: exahm, dropCost: drop, minID: id}
}

func emptyFixed(n int) ([]int, []map[string]bool) {
	used := make([]int, n)
	progs := make([]map[string]bool, n)
	for i := range progs {
		progs[i] = map[string]bool{}
	}
	return used, progs
}

// checkCapacity verifies the only hard constraint: no slot is over capacity.
func checkCapacity(t *testing.T, units []*preplanUnit, slots []*preplanSlot, assign []int) {
	t.Helper()
	used := make([]int, len(slots))
	for u, s := range assign {
		if s >= 0 {
			used[s] += units[u].seats
		}
	}
	for s := range slots {
		if used[s] > slots[s].capacity {
			t.Errorf("slot %d over capacity: %d > %d", s, used[s], slots[s].capacity)
		}
	}
}

// Program overlap is only a soft cost now, so even many exams that all share a program
// must still be placed as long as the seat capacity allows it.
func TestSolvePreplanPlacesAllWhenCapacityAllows(t *testing.T) {
	// sum 290 into 3 bins of 108, e.g. {90} {60,40} {50,30,20} — feasible
	units := []*preplanUnit{
		unit(1, 90, false, "IF"),
		unit(2, 60, true, "IF"),
		unit(3, 50, false, "IF", "IC"),
		unit(4, 40, false, "IF", "IC"),
		unit(5, 30, false, "IF"),
		unit(6, 20, true, "IF"),
	}
	slots := []*preplanSlot{
		{day: 1, slotNo: 1, capacity: 108},
		{day: 2, slotNo: 1, capacity: 108},
		{day: 3, slotNo: 1, capacity: 108},
	}
	fu, fp := emptyFixed(len(slots))

	assign := solvePreplan(units, slots, fu, fp)
	if n := countUnplaced(assign); n != 0 {
		t.Fatalf("expected all units placed (capacity allows), %d unplaced: %v", n, assign)
	}
	checkCapacity(t, units, slots, assign)
}

// When capacity forces a drop, EXaHM and large SEB must survive; the smallest SEB is
// the one left without a slot.
func TestSolvePreplanDropsSmallestWhenTight(t *testing.T) {
	// one slot, capacity 100; total demand 110 → exactly one unit must drop.
	units := []*preplanUnit{
		unit(1, 60, true, "P1"),
		unit(2, 40, false, "P2"),
		unit(3, 10, false, "P3"),
	}
	slots := []*preplanSlot{{day: 1, slotNo: 1, capacity: 100}}
	fu, fp := emptyFixed(len(slots))

	assign := solvePreplan(units, slots, fu, fp)
	checkCapacity(t, units, slots, assign)
	if assign[0] < 0 {
		t.Errorf("EXaHM unit must not be dropped: %v", assign)
	}
	if assign[1] < 0 {
		t.Errorf("large SEB (40) must not be dropped: %v", assign)
	}
	if assign[2] >= 0 {
		t.Errorf("smallest SEB (10) should be the dropped one: %v", assign)
	}
}

// Two exams of the same program should go to different slots when capacity allows.
func TestSolvePreplanSeparatesSameProgramBySlot(t *testing.T) {
	units := []*preplanUnit{unit(1, 10, false, "IF"), unit(2, 10, false, "IF")}
	slots := []*preplanSlot{
		{day: 1, slotNo: 1, capacity: 100},
		{day: 1, slotNo: 2, capacity: 100},
	}
	fu, fp := emptyFixed(len(slots))

	assign := solvePreplan(units, slots, fu, fp)
	checkCapacity(t, units, slots, assign)
	if countUnplaced(assign) != 0 {
		t.Fatalf("both units should be placed: %v", assign)
	}
	if assign[0] == assign[1] {
		t.Errorf("same-program units should be in different slots: %v", assign)
	}
}

// A unit restricted to certain slots (MUC.DAI exams → MUC.DAI slots) must land in one
// of them, even when other slots have more room.
func TestSolvePreplanRespectsAllowedSlots(t *testing.T) {
	mucdai := unit(1, 70, true, "DE", "GS", "ID")
	mucdai.allowedSlots = map[int]bool{2: true} // only slot index 2 is allowed
	units := []*preplanUnit{
		unit(2, 60, false, "IF"),
		unit(3, 60, false, "IF"),
		mucdai,
	}
	slots := []*preplanSlot{
		{day: 1, slotNo: 1, capacity: 108},
		{day: 2, slotNo: 1, capacity: 108},
		{day: 3, slotNo: 1, capacity: 108}, // the only allowed slot for the MUC.DAI unit
	}
	fu, fp := emptyFixed(len(slots))

	assign := solvePreplan(units, slots, fu, fp)
	checkCapacity(t, units, slots, assign)
	if assign[2] != 2 {
		t.Errorf("MUC.DAI unit must be in its only allowed slot (2), got %d", assign[2])
	}
	if countUnplaced(assign) != 0 {
		t.Errorf("all units should be placed: %v", assign)
	}
}

// An explicit "nicht gleichzeitig" pair (no shared program) is spread apart: different
// days when possible, and never the same slot.
func TestSolvePreplanExplicitConflictSpreads(t *testing.T) {
	a := unit(1, 10, false, "AA")
	b := unit(2, 10, false, "BB")
	a.conflicts = map[int]int{1: preplanExplicitConflictWeight}
	b.conflicts = map[int]int{0: preplanExplicitConflictWeight}
	units := []*preplanUnit{a, b}
	slots := []*preplanSlot{
		{day: 1, slotNo: 1, capacity: 100},
		{day: 1, slotNo: 2, capacity: 100},
		{day: 2, slotNo: 1, capacity: 100},
	}
	fu, fp := emptyFixed(len(slots))

	assign := solvePreplan(units, slots, fu, fp)
	if countUnplaced(assign) != 0 {
		t.Fatalf("both should be placed: %v", assign)
	}
	if slots[assign[0]].day == slots[assign[1]].day {
		t.Errorf("explicit-conflict units should be on different days: %v", assign)
	}
}

// Two same-program units marked compatible must NOT be forced apart: with only one slot
// that has room for both, they may share it (no spreading penalty cancels placement).
func TestSolvePreplanCompatibleMayShareSlot(t *testing.T) {
	a := unit(1, 30, false, "WD", "WT")
	b := unit(2, 30, false, "WD", "WT")
	a.compatible = map[int]bool{1: true}
	b.compatible = map[int]bool{0: true}
	units := []*preplanUnit{a, b}
	// one roomy slot + one tiny slot: without the exemption the solver would push b into
	// the tiny slot (and fail capacity); the exemption lets both share the roomy one.
	slots := []*preplanSlot{
		{day: 1, slotNo: 1, capacity: 100},
		{day: 1, slotNo: 2, capacity: 10},
	}
	fu, fp := emptyFixed(len(slots))

	assign := solvePreplan(units, slots, fu, fp)
	checkCapacity(t, units, slots, assign)
	if countUnplaced(assign) != 0 {
		t.Fatalf("both should be placed: %v", assign)
	}
	if assign[0] != 0 || assign[1] != 0 {
		t.Errorf("compatible same-program units should share the roomy slot 0: %v", assign)
	}
}

func TestProximityPenalty(t *testing.T) {
	s := func(d, n int) *preplanSlot { return &preplanSlot{day: d, slotNo: n} }
	cases := []struct {
		a, b *preplanSlot
		want int
	}{
		{s(1, 1), s(1, 1), 100}, // same slot → full
		{s(1, 1), s(1, 2), 90},  // same day, 1 apart
		{s(1, 1), s(1, 5), 60},  // same day, 4 apart
		{s(1, 1), s(2, 1), 0},   // different day → 0
		{s(1, 1), s(3, 3), 0},   // far → 0
	}
	for _, c := range cases {
		if got := proximityPenalty(c.a, c.b, 100); got != c.want {
			t.Errorf("proximityPenalty(%v,%v)=%d want %d", c.a, c.b, got, c.want)
		}
	}
}

// Same program → different days when slots are spread over days.
func TestSolvePreplanSpreadsAcrossDays(t *testing.T) {
	units := []*preplanUnit{unit(1, 10, false, "IF"), unit(2, 10, false, "IF")}
	// two slots on day 1, one on day 2; the two IF units should land on different days.
	slots := []*preplanSlot{
		{day: 1, slotNo: 1, capacity: 100},
		{day: 1, slotNo: 2, capacity: 100},
		{day: 2, slotNo: 1, capacity: 100},
	}
	fu, fp := emptyFixed(len(slots))

	assign := solvePreplan(units, slots, fu, fp)
	checkCapacity(t, units, slots, assign)
	if countUnplaced(assign) != 0 {
		t.Fatalf("both units should be placed: %v", assign)
	}
	if slots[assign[0]].day == slots[assign[1]].day {
		t.Errorf("the two same-program units should be on different days: %v", assign)
	}
}
