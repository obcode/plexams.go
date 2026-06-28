package plexams

import "testing"

func progSet(ps ...string) map[string]bool {
	m := make(map[string]bool, len(ps))
	for _, p := range ps {
		m[p] = true
	}
	return m
}

// checkAssignment verifies the hard constraints: no study program twice in a slot,
// and no slot over capacity.
func checkAssignment(t *testing.T, units []*preplanUnit, slots []*preplanSlot, assign []int) {
	t.Helper()
	used := make([]int, len(slots))
	progs := make([]map[string]bool, len(slots))
	for s := range slots {
		progs[s] = map[string]bool{}
	}
	for u, s := range assign {
		if s < 0 {
			continue
		}
		used[s] += units[u].seats
		for p := range units[u].programs {
			if progs[s][p] {
				t.Errorf("program %q appears twice in slot %d", p, s)
			}
			progs[s][p] = true
		}
	}
	for s := range slots {
		if used[s] > slots[s].capacity {
			t.Errorf("slot %d over capacity: %d > %d", s, used[s], slots[s].capacity)
		}
	}
}

func emptyFixed(n int) ([]int, []map[string]bool) {
	used := make([]int, n)
	progs := make([]map[string]bool, n)
	for i := range progs {
		progs[i] = map[string]bool{}
	}
	return used, progs
}

// Crown graph S3: bipartite (2-colourable), but first-fit by index needs 3 colours
// and would strand 2 vertices when only 2 slots exist. DSATUR must place all 6.
func TestSolvePreplanCrownGraphAllPlaced(t *testing.T) {
	units := []*preplanUnit{
		{members: []int{0}, seats: 1, programs: progSet("a1b2", "a1b3"), minID: 1, dropCost: preplanDropBase + 1},
		{members: []int{1}, seats: 1, programs: progSet("a2b1", "a2b3"), minID: 2, dropCost: preplanDropBase + 1},
		{members: []int{2}, seats: 1, programs: progSet("a3b1", "a3b2"), minID: 3, dropCost: preplanDropBase + 1},
		{members: []int{3}, seats: 1, programs: progSet("a2b1", "a3b1"), minID: 4, dropCost: preplanDropBase + 1},
		{members: []int{4}, seats: 1, programs: progSet("a1b2", "a3b2"), minID: 5, dropCost: preplanDropBase + 1},
		{members: []int{5}, seats: 1, programs: progSet("a1b3", "a2b3"), minID: 6, dropCost: preplanDropBase + 1},
	}
	slots := []*preplanSlot{
		{day: 1, slotNo: 1, capacity: 100},
		{day: 2, slotNo: 1, capacity: 100},
	}
	fu, fp := emptyFixed(len(slots))

	assign := solvePreplan(units, slots, fu, fp)
	if n := countUnplaced(assign); n != 0 {
		t.Fatalf("expected all 6 units placed in 2 slots, %d unplaced: %v", n, assign)
	}
	checkAssignment(t, units, slots, assign)
}

// When capacity forces a drop, EXaHM and large SEB must survive; the smallest SEB is
// the one left without a slot.
func TestSolvePreplanDropsSmallestWhenTight(t *testing.T) {
	// one slot, capacity 100; total demand 110 → exactly one unit must drop.
	units := []*preplanUnit{
		{members: []int{0}, seats: 60, programs: progSet("P1"), hasExahm: true, minID: 1, dropCost: preplanDropBase + 60 + preplanExahmKeep},
		{members: []int{1}, seats: 40, programs: progSet("P2"), minID: 2, dropCost: preplanDropBase + 40},
		{members: []int{2}, seats: 10, programs: progSet("P3"), minID: 3, dropCost: preplanDropBase + 10},
	}
	slots := []*preplanSlot{{day: 1, slotNo: 1, capacity: 100}}
	fu, fp := emptyFixed(len(slots))

	assign := solvePreplan(units, slots, fu, fp)
	checkAssignment(t, units, slots, assign)
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

// Same program → different days when slots are spread over days.
func TestSolvePreplanSpreadsAcrossDays(t *testing.T) {
	units := []*preplanUnit{
		{members: []int{0}, seats: 10, programs: progSet("IF"), minID: 1, dropCost: preplanDropBase + 10},
		{members: []int{1}, seats: 10, programs: progSet("IF"), minID: 2, dropCost: preplanDropBase + 10},
	}
	// two slots on day 1, one on day 2; the two IF units share a program (different
	// slots) and should land on different days (one on day 1, one on day 2).
	slots := []*preplanSlot{
		{day: 1, slotNo: 1, capacity: 100},
		{day: 1, slotNo: 2, capacity: 100},
		{day: 2, slotNo: 1, capacity: 100},
	}
	fu, fp := emptyFixed(len(slots))

	assign := solvePreplan(units, slots, fu, fp)
	checkAssignment(t, units, slots, assign)
	if countUnplaced(assign) != 0 {
		t.Fatalf("both units should be placed: %v", assign)
	}
	day0, day1 := slots[assign[0]].day, slots[assign[1]].day
	if day0 == day1 {
		t.Errorf("the two same-program units should be on different days, both on day %d", day0)
	}
}
