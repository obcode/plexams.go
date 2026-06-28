package plexams

import (
	"math"
	"math/rand"
	"sort"
)

// The pre-plan assignment distributes pre-exams (or same-slot groups) over the booked
// Anny slots. The only HARD limit is each slot's seat capacity; same-slot groups stay
// together (they are pre-merged into one unit). Putting two exams of the same study
// program into the same slot is allowed but discouraged (a SOFT cost) — some exams of
// the same program may even have to run at the same time, others should be spread over
// slots and days. We solve it with a DSATUR-style constructive pass (most-constrained,
// then most-important unit first) and a simulated-annealing repair that ejects exams to
// free capacity, mirroring the invigilation planner.

const (
	// preplanDropBase is the cost of leaving any unit without a slot; it dominates the
	// soft program-spread cost so the solver places as many exams as possible first.
	preplanDropBase = 10000
	// preplanExahmKeep is added to an EXaHM unit's drop cost so EXaHM (and, via the
	// seat term, large SEB) are never dropped while anything smaller could be.
	preplanExahmKeep = 1000000
	// preplanSameSlotProgWeight penalises the same study program twice in one slot.
	preplanSameSlotProgWeight = 50
	// preplanSameDayProgWeight penalises the same study program twice on one day
	// (in different slots) — a weaker pull than the same-slot one.
	preplanSameDayProgWeight = 5

	preplanSAIterations = 20000
	preplanSAStartTemp  = 20000.0
	preplanSAEndTemp    = 1.0
	preplanEjectDepth   = 3 // max units kicked out of a slot to free capacity
)

// preplanSlot is a candidate MUC.DAI slot that already has Anny rooms booked.
type preplanSlot struct {
	day, slotNo int
	capacity    int // usable seats (~90% of the booked physical seats)
}

// preplanUnit is a set of pre-exams that must share a slot (same-slot), treated as one
// indivisible item.
type preplanUnit struct {
	members  []int // indices into the pre-exam slice
	seats    int
	programs map[string]bool
	hasExahm bool
	dropCost int
	minID    int
	// allowedSlots restricts the unit to a subset of slot indices (nil = any slot).
	// Used so MUC.DAI exams (programs DE/GS/ID) only land in MUC.DAI slots.
	allowedSlots map[int]bool
}

// solvePreplan distributes the units over the candidate slots. fixedUsed/fixedProgs
// hold the seats and programs of pinned exams already occupying each slot. It returns,
// per unit, the slot index it was assigned to, or -1 when it could not be placed.
func solvePreplan(units []*preplanUnit, slots []*preplanSlot, fixedUsed []int, fixedProgs []map[string]bool) []int {
	n := len(units)
	assign := make([]int, n)
	for i := range assign {
		assign[i] = -1
	}
	if n == 0 || len(slots) == 0 {
		return assign
	}

	// seats used + non-fixed occupants per slot for an assignment
	occupancy := func(a []int) (used []int, occ [][]int) {
		used = make([]int, len(slots))
		occ = make([][]int, len(slots))
		copy(used, fixedUsed)
		for u, s := range a {
			if s < 0 {
				continue
			}
			used[s] += units[u].seats
			occ[s] = append(occ[s], u)
		}
		return
	}

	// program counts per slot and per day, including the fixed occupants (as one each)
	counts := func(a []int) (slotCnt map[string]map[int]int, dayCnt map[string]map[int]int) {
		slotCnt = map[string]map[int]int{}
		dayCnt = map[string]map[int]int{}
		bump := func(m map[string]map[int]int, prog string, key int) {
			if m[prog] == nil {
				m[prog] = map[int]int{}
			}
			m[prog][key]++
		}
		for s := range slots {
			for prog := range fixedProgs[s] {
				bump(slotCnt, prog, s)
				bump(dayCnt, prog, slots[s].day)
			}
		}
		for u, s := range a {
			if s < 0 {
				continue
			}
			for prog := range units[u].programs {
				bump(slotCnt, prog, s)
				bump(dayCnt, prog, slots[s].day)
			}
		}
		return
	}

	fits := func(u, s int, used []int) bool {
		if units[u].allowedSlots != nil && !units[u].allowedSlots[s] {
			return false
		}
		return used[s]+units[u].seats <= slots[s].capacity
	}

	// --- DSATUR constructive pass: most constrained / most important unit first ---
	done := make([]bool, n)
	for remaining := n; remaining > 0; remaining-- {
		used, _ := occupancy(assign)

		best := -1
		var bestFeasible []int
		for u := 0; u < n; u++ {
			if done[u] {
				continue
			}
			fs := make([]int, 0, len(slots))
			for s := range slots {
				if fits(u, s, used) {
					fs = append(fs, s)
				}
			}
			if best == -1 || dsaturBefore(units[u], len(fs), units[best], len(bestFeasible)) {
				best, bestFeasible = u, fs
			}
		}

		done[best] = true
		if len(bestFeasible) == 0 {
			continue // no slot has room right now → leave for the SA repair
		}
		assign[best] = chooseSlot(units[best], bestFeasible, assign, units, slots, used, counts)
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
		total += softProgramCost(a, units, slots, counts)
		return total
	}

	rng := rand.New(rand.NewSource(1)) //nolint:gosec // deterministic, not security relevant
	cur := append([]int(nil), assign...)
	curCost := cost(cur)
	best := append([]int(nil), cur...)
	bestCost := curCost

	for it := 0; it < preplanSAIterations; it++ {
		cand := append([]int(nil), cur...)
		if !proposeMove(cand, rng, units, slots, occupancy) {
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

// dsaturBefore reports whether unit u (with fu feasible slots) should be placed before
// unit v (with fv feasible slots): fewest options first (capacity-tight units), then
// higher priority (EXaHM/large via the drop cost), then smaller id.
func dsaturBefore(u *preplanUnit, fu int, v *preplanUnit, fv int) bool {
	if fu != fv {
		return fu < fv
	}
	if u.dropCost != v.dropCost {
		return u.dropCost > v.dropCost
	}
	return u.minID < v.minID
}

// chooseSlot picks, among the feasible slots, the one that adds the least
// program-overlap cost (same program in slot, then same program on day), tie-broken by
// most free capacity.
func chooseSlot(u *preplanUnit, feasibleSlots []int, assign []int, units []*preplanUnit, slots []*preplanSlot,
	used []int, counts func([]int) (map[string]map[int]int, map[string]map[int]int)) int {
	slotCnt, dayCnt := counts(assign)
	best, bestPenalty, bestFree := -1, math.MaxInt, -1
	for _, s := range feasibleSlots {
		ps, pd := 0, 0
		for prog := range u.programs {
			ps += slotCnt[prog][s]
			pd += dayCnt[prog][slots[s].day]
		}
		penalty := preplanSameSlotProgWeight*ps + preplanSameDayProgWeight*(pd-ps)
		free := slots[s].capacity - used[s]
		if best == -1 || penalty < bestPenalty || (penalty == bestPenalty && free > bestFree) {
			best, bestPenalty, bestFree = s, penalty, free
		}
	}
	return best
}

// proposeMove relocates a random unit to a random slot, ejecting up to
// preplanEjectDepth occupants (smallest first) to free the needed capacity. Fixed
// occupancy is in fixedUsed and is never ejected. The move keeps the slot within
// capacity; program overlaps are allowed (they only cost via the soft term).
func proposeMove(a []int, rng *rand.Rand, units []*preplanUnit, slots []*preplanSlot,
	occupancy func([]int) ([]int, [][]int)) bool {
	n := len(units)
	u := rng.Intn(n)
	s := rng.Intn(len(slots))
	if a[u] == s {
		return false
	}
	if units[u].allowedSlots != nil && !units[u].allowedSlots[s] {
		return false
	}
	used, occ := occupancy(a)
	avail := slots[s].capacity - used[s]
	need := units[u].seats
	if avail >= need {
		a[u] = s
		return true
	}

	cand := append([]int(nil), occ[s]...)
	sort.Slice(cand, func(i, j int) bool { return units[cand[i]].seats < units[cand[j]].seats })
	ejected := make([]int, 0, preplanEjectDepth)
	for _, v := range cand {
		if avail >= need {
			break
		}
		if len(ejected) >= preplanEjectDepth {
			return false
		}
		ejected = append(ejected, v)
		avail += units[v].seats
	}
	if avail < need {
		return false
	}
	for _, v := range ejected {
		a[v] = -1
	}
	a[u] = s
	return true
}

// softProgramCost sums the program-overlap penalties of an assignment: same study
// program twice in a slot, and (more weakly) same program twice on a day.
func softProgramCost(a []int, units []*preplanUnit, slots []*preplanSlot,
	counts func([]int) (map[string]map[int]int, map[string]map[int]int)) float64 {
	slotCnt, dayCnt := counts(a)
	pairs := func(m map[string]map[int]int) int {
		total := 0
		for _, byKey := range m {
			for _, c := range byKey {
				total += c * (c - 1) / 2
			}
		}
		return total
	}
	slotPairs := pairs(slotCnt)
	dayPairs := pairs(dayCnt)
	return float64(preplanSameSlotProgWeight*slotPairs + preplanSameDayProgWeight*(dayPairs-slotPairs))
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
