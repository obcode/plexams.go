package plexams

import (
	"math"
	"math/rand"
)

// The pre-plan assignment is a graph-colouring problem with bin capacities: each
// pre-exam (or same-slot group) is a node, two pre-exams that share a study program
// must not land in the same slot, and every booked Anny slot is a "colour" with a
// seat limit. We solve it with a DSATUR constructive pass (most-constrained unit
// first) and, only if that leaves something unplaced, a simulated-annealing repair
// that ejects less important exams to make room — mirroring the invigilation planner.

const (
	// preplanDropBase is the cost of leaving any unit without a slot; it dominates
	// the soft day-spread cost so the solver places as many exams as possible first.
	preplanDropBase = 10000
	// preplanExahmKeep is added to an EXaHM unit's drop cost so EXaHM (and, via the
	// seat term, large SEB) are never dropped while anything smaller could be.
	preplanExahmKeep = 1000000
	// preplanDaySpreadCost penalises a study program appearing twice on the same day.
	preplanDaySpreadCost = 1

	preplanSAIterations = 20000
	preplanSAStartTemp  = 20000.0
	preplanSAEndTemp    = 1.0
	preplanEjectDepth   = 2 // max units kicked out of a slot to place a more important one
)

// preplanSlot is a candidate MUC.DAI slot that already has Anny rooms booked.
type preplanSlot struct {
	day, slotNo int
	capacity    int // usable seats (~90% of the booked physical seats)
}

// preplanUnit is a set of pre-exams that must share a slot (same-slot), treated as
// one indivisible item.
type preplanUnit struct {
	members  []int // indices into the pre-exam slice
	seats    int
	programs map[string]bool
	hasExahm bool
	dropCost int
	minID    int
}

// solvePreplan distributes the units over the candidate slots. fixedUsed/fixedProgs
// hold the seats and programs of pinned exams already occupying each slot. It
// returns, per unit, the slot index it was assigned to, or -1 when it could not be
// placed.
func solvePreplan(units []*preplanUnit, slots []*preplanSlot, fixedUsed []int, fixedProgs []map[string]bool) []int {
	n := len(units)
	assign := make([]int, n)
	for i := range assign {
		assign[i] = -1
	}
	if n == 0 || len(slots) == 0 {
		return assign
	}

	// occupancy (seats + programs + non-fixed occupants) per slot for an assignment
	occupancy := func(a []int) (used []int, progs []map[string]bool, occ [][]int) {
		used = make([]int, len(slots))
		progs = make([]map[string]bool, len(slots))
		occ = make([][]int, len(slots))
		for s := range slots {
			used[s] = fixedUsed[s]
			progs[s] = map[string]bool{}
			for p := range fixedProgs[s] {
				progs[s][p] = true
			}
		}
		for u, s := range a {
			if s < 0 {
				continue
			}
			used[s] += units[u].seats
			for p := range units[u].programs {
				progs[s][p] = true
			}
			occ[s] = append(occ[s], u)
		}
		return
	}

	feasible := func(u, s int, used []int, progs []map[string]bool) bool {
		if used[s]+units[u].seats > slots[s].capacity {
			return false
		}
		for p := range units[u].programs {
			if progs[s][p] {
				return false
			}
		}
		return true
	}

	// --- DSATUR constructive pass: place the most constrained unit first ---
	done := make([]bool, n)
	for remaining := n; remaining > 0; remaining-- {
		used, progs, _ := occupancy(assign)

		best := -1
		var bestFeasible []int
		for u := 0; u < n; u++ {
			if done[u] {
				continue
			}
			fs := make([]int, 0, len(slots))
			for s := range slots {
				if feasible(u, s, used, progs) {
					fs = append(fs, s)
				}
			}
			if best == -1 || dsaturBefore(units[u], len(fs), units[best], len(bestFeasible)) {
				best, bestFeasible = u, fs
			}
		}

		done[best] = true
		if len(bestFeasible) == 0 {
			continue // no slot fits right now → leave for the SA repair
		}
		assign[best] = chooseSlot(units[best], bestFeasible, assign, units, slots, fixedProgs)
	}

	// --- simulated-annealing repair, only if something is still unplaced ---
	if countUnplaced(assign) == 0 {
		return assign
	}

	cost := func(a []int) float64 {
		total := 0.0
		for u, s := range a {
			if s < 0 {
				total += float64(units[u].dropCost)
			}
		}
		total += preplanDaySpreadCost * float64(daySpread(a, units, slots, fixedProgs))
		return total
	}

	rng := rand.New(rand.NewSource(1)) //nolint:gosec // deterministic, not security relevant
	cur := append([]int(nil), assign...)
	curCost := cost(cur)
	best := append([]int(nil), cur...)
	bestCost := curCost

	for it := 0; it < preplanSAIterations; it++ {
		cand := append([]int(nil), cur...)
		if !proposeMove(cand, rng, units, slots, fixedUsed, fixedProgs, occupancy, feasible) {
			continue
		}
		c := cost(cand)
		t := preplanSAStartTemp * math.Pow(preplanSAEndTemp/preplanSAStartTemp, float64(it)/float64(preplanSAIterations))
		if c <= curCost || rng.Float64() < math.Exp(-(c-curCost)/t) {
			cur, curCost = cand, c
			if curCost < bestCost {
				best, bestCost = append([]int(nil), cur...), curCost
			}
		}
	}
	return best
}

// dsaturBefore reports whether unit u (with fu feasible slots) should be coloured
// before unit v (with fv feasible slots): fewest options first, then higher priority
// (EXaHM/large), then smaller id.
func dsaturBefore(u *preplanUnit, fu int, v *preplanUnit, fv int) bool {
	if fu != fv {
		return fu < fv
	}
	if u.dropCost != v.dropCost {
		return u.dropCost > v.dropCost
	}
	return u.minID < v.minID
}

// chooseSlot picks, among the feasible slots, the one that best spreads the unit's
// study programs across days, then the one with the most free capacity.
func chooseSlot(u *preplanUnit, feasibleSlots []int, assign []int, units []*preplanUnit, slots []*preplanSlot, fixedProgs []map[string]bool) int {
	// days already used by the unit's programs (current assignment + fixed)
	dayUsed := make(map[int]int)
	addDay := func(progs map[string]bool, day int) {
		for p := range progs {
			if u.programs[p] {
				dayUsed[day]++
			}
		}
	}
	for s := range slots {
		addDay(fixedProgs[s], slots[s].day)
	}
	for w, s := range assign {
		if s >= 0 {
			addDay(units[w].programs, slots[s].day)
		}
	}
	used := slotUsage(assign, units, slots)

	best, bestPenalty, bestFree := -1, 0, -1
	for _, s := range feasibleSlots {
		penalty := dayUsed[slots[s].day]
		free := slots[s].capacity - used[s]
		if best == -1 || penalty < bestPenalty || (penalty == bestPenalty && free > bestFree) {
			best, bestPenalty, bestFree = s, penalty, free
		}
	}
	return best
}

// proposeMove relocates a random unit to a random slot, ejecting up to
// preplanEjectDepth conflicting (non-fixed) units to make room. Fixed occupants are
// never ejected. The move keeps the slot hard-feasible (capacity + program-disjoint).
func proposeMove(a []int, rng *rand.Rand, units []*preplanUnit, slots []*preplanSlot,
	fixedUsed []int, fixedProgs []map[string]bool,
	occupancy func([]int) ([]int, []map[string]bool, [][]int),
	feasible func(int, int, []int, []map[string]bool) bool,
) bool {
	n := len(units)
	u := rng.Intn(n)
	s := rng.Intn(len(slots))
	if a[u] == s {
		return false
	}

	used, _, occ := occupancy(a)

	// never share a program with a pinned occupant of s
	for p := range units[u].programs {
		if fixedProgs[s][p] {
			return false
		}
	}

	// conflicting non-fixed occupants that would have to leave
	conflicts := make([]int, 0)
	for _, v := range occ[s] {
		if v == u {
			continue
		}
		if shareProgram(units[u], units[v]) {
			conflicts = append(conflicts, v)
		}
	}
	if len(conflicts) > preplanEjectDepth {
		return false
	}
	freed := 0
	for _, v := range conflicts {
		freed += units[v].seats
	}
	if used[s]-freed+units[u].seats > slots[s].capacity {
		return false
	}

	for _, v := range conflicts {
		a[v] = -1
	}
	a[u] = s
	return true
}

func shareProgram(u, v *preplanUnit) bool {
	for p := range u.programs {
		if v.programs[p] {
			return true
		}
	}
	return false
}

func countUnplaced(a []int) int {
	n := 0
	for _, s := range a {
		if s < 0 {
			n++
		}
	}
	return n
}

func slotUsage(a []int, units []*preplanUnit, slots []*preplanSlot) []int {
	used := make([]int, len(slots))
	for u, s := range a {
		if s >= 0 {
			used[s] += units[u].seats
		}
	}
	return used
}

// daySpread counts, per study program, how often it appears more than once on the
// same day (lower is better). Fixed occupants are included via fixedProgs.
func daySpread(a []int, units []*preplanUnit, slots []*preplanSlot, fixedProgs []map[string]bool) int {
	perProgramDay := make(map[string]map[int]int)
	add := func(progs map[string]bool, day int) {
		for p := range progs {
			if perProgramDay[p] == nil {
				perProgramDay[p] = map[int]int{}
			}
			perProgramDay[p][day]++
		}
	}
	for s := range slots {
		add(fixedProgs[s], slots[s].day)
	}
	for u, s := range a {
		if s >= 0 {
			add(units[u].programs, slots[s].day)
		}
	}
	total := 0
	for _, days := range perProgramDay {
		for _, c := range days {
			if c > 1 {
				total += c - 1
			}
		}
	}
	return total
}
