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
	// preplanProgramConflictWeight is the base penalty for two exams that share a study
	// program; preplanExplicitConflictWeight for an explicit "nicht gleichzeitig" pair.
	// Both are soft (well below preplanDropBase) and scaled by temporal proximity, so
	// the solver spreads such exams (different days, else max slot distance) but never
	// leaves one unplaced just to separate them.
	preplanProgramConflictWeight  = 100
	preplanExplicitConflictWeight = 1000

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
	// conflicts maps another unit index to the penalty weight for placing the two
	// close in time (shared study program, or an explicit "nicht gleichzeitig" pair).
	conflicts map[int]int
	// compatible holds unit indices that are exempt from the program-based spreading
	// (an explicit "darf zusammen / direkt nacheinander" pair).
	compatible map[int]bool
}

// proximityPenalty is the soft cost of placing two conflicting units in slots a and b:
// p0 when in the same slot, scaled down with temporal distance, and 0 once they are on
// different days (different days are always "far enough"). Within a day it rewards the
// maximum slot distance.
func proximityPenalty(a, b *preplanSlot, p0 int) int {
	dist := absInt(a.day-b.day)*10 + absInt(a.slotNo-b.slotNo)
	if dist >= 10 {
		return 0
	}
	return p0 * (10 - dist) / 10
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
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

	// units sharing a study program conflict softly; merge with any explicit conflicts
	// already set by the caller (those use the stronger explicit weight and win).
	for i := range units {
		if units[i].conflicts == nil {
			units[i].conflicts = map[int]int{}
		}
	}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if units[i].compatible[j] {
				continue // explicitly allowed to share a slot / be adjacent
			}
			if shareProgram(units[i], units[j]) && units[i].conflicts[j] < preplanProgramConflictWeight {
				units[i].conflicts[j] = preplanProgramConflictWeight
				units[j].conflicts[i] = preplanProgramConflictWeight
			}
		}
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
		assign[best] = chooseSlot(best, bestFeasible, assign, units, slots, used, fixedProgs)
	}

	// --- simulated-annealing pass: place any remaining units AND optimise the soft
	// proximity cost (spread conflicting exams across days / max slot distance) ---
	cost := func(a []int) float64 {
		total := 0.0
		for u, s := range a {
			if s < 0 {
				total += float64(units[u].dropCost)
				continue
			}
			// proximity to fixed occupants that share a study program
			for f := range slots {
				if len(fixedProgs[f]) > 0 && shareProgramSet(units[u].programs, fixedProgs[f]) {
					total += float64(proximityPenalty(slots[s], slots[f], preplanProgramConflictWeight))
				}
			}
			// conflicting unit pairs (counted once via v > u)
			for v, p0 := range units[u].conflicts {
				if v > u && a[v] >= 0 {
					total += float64(proximityPenalty(slots[s], slots[a[v]], p0))
				}
			}
		}
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

// chooseSlot picks, among the feasible slots, the one that adds the least conflict
// proximity cost (conflicting units / shared-program fixed occupants placed close in
// time), tie-broken by most free capacity.
func chooseSlot(u int, feasibleSlots []int, assign []int, units []*preplanUnit, slots []*preplanSlot,
	used []int, fixedProgs []map[string]bool) int {
	best, bestPenalty, bestFree := -1, math.MaxInt, -1
	for _, s := range feasibleSlots {
		penalty := conflictCostAt(u, s, assign, units, slots, fixedProgs)
		free := slots[s].capacity - used[s]
		if best == -1 || penalty < bestPenalty || (penalty == bestPenalty && free > bestFree) {
			best, bestPenalty, bestFree = s, penalty, free
		}
	}
	return best
}

// conflictCostAt is the proximity cost added by placing unit u in slot s, given the
// current assignment: against every already-placed conflicting unit and every fixed
// occupant that shares a study program.
func conflictCostAt(u, s int, assign []int, units []*preplanUnit, slots []*preplanSlot, fixedProgs []map[string]bool) int {
	pen := 0
	for v, p0 := range units[u].conflicts {
		if assign[v] >= 0 {
			pen += proximityPenalty(slots[s], slots[assign[v]], p0)
		}
	}
	for f := range slots {
		if len(fixedProgs[f]) > 0 && shareProgramSet(units[u].programs, fixedProgs[f]) {
			pen += proximityPenalty(slots[s], slots[f], preplanProgramConflictWeight)
		}
	}
	return pen
}

func shareProgram(u, v *preplanUnit) bool {
	return shareProgramSet(u.programs, v.programs)
}

func shareProgramSet(progs, set map[string]bool) bool {
	for p := range progs {
		if set[p] {
			return true
		}
	}
	return false
}

// proposeMove relocates a random unit to a random slot, ejecting up to
// preplanEjectDepth occupants (smallest first) to free the needed capacity. Fixed
// occupancy is in fixedUsed and is never ejected. The move keeps the slot within
// capacity; program overlaps are allowed (they only cost via the soft term).
func proposeMove(a []int, rng *rand.Rand, units []*preplanUnit, slots []*preplanSlot,
	occupancy func([]int) ([]int, [][]int)) bool {
	n := len(units)

	// half the time: swap two placed units' slots (keeps everyone placed; explores the
	// soft proximity cost without freeing/creating capacity gaps).
	if rng.Float64() < 0.5 {
		u, v := rng.Intn(n), rng.Intn(n)
		if u == v || a[u] < 0 || a[v] < 0 || a[u] == a[v] {
			return false
		}
		su, sv := a[u], a[v]
		if units[u].allowedSlots != nil && !units[u].allowedSlots[sv] {
			return false
		}
		if units[v].allowedSlots != nil && !units[v].allowedSlots[su] {
			return false
		}
		used, _ := occupancy(a)
		if used[su]-units[u].seats+units[v].seats > slots[su].capacity {
			return false
		}
		if used[sv]-units[v].seats+units[u].seats > slots[sv].capacity {
			return false
		}
		a[u], a[v] = sv, su
		return true
	}

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

func countUnplaced(a []int) int {
	n := 0
	for _, s := range a {
		if s < 0 {
			n++
		}
	}
	return n
}
