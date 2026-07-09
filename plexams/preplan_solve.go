package plexams

import (
	"math"
	"math/rand"
	"sort"
	"time"

	"github.com/obcode/plexams.go/plexams/optimize"
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
	// preplanSmallSebDrop is the drop cost of a small SEB that fits a single R-building lab:
	// low enough that the solver yields its slot to any must-place exam (which have a drop
	// cost ≥ preplanDropBase) and prefers leaving it for the R-building over displacing
	// others, yet well above the soft proximity penalties so it IS placed into genuinely
	// free booked capacity when available.
	preplanSmallSebDrop = 2000
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

// preplanSlot is a candidate MUC.DAI slot that already has Anny rooms booked,
// identified by its absolute start time (no day/slot ordinals).
type preplanSlot struct {
	start    time.Time
	capacity int // usable seats (~90% of the booked physical seats)
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
	// window parameters (max over members) for room packing: exam duration and the
	// setup/teardown buffer. A unit's exams must fit into DISTINCT booked rooms covering
	// [start-pre, start+dur+post]; see packExamsIntoRooms.
	dur, pre, post time.Duration
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
// p0 when at the same start time, scaled down with the time gap on the same calendar
// day, and 0 once they are on different days (different days are always "far enough").
// Within a day it rewards the maximum time distance (a full working day ~8h apart is
// treated as far enough).
func proximityPenalty(a, b *preplanSlot, p0 int) int {
	ay, am, ad := a.start.Date()
	by, bm, bd := b.start.Date()
	if ay != by || am != bm || ad != bd {
		return 0 // different calendar day is always "far enough"
	}
	// same calendar day: full at the same start time, decreasing with the time gap, but
	// never zero (a same-day pair must always cost more than a different-day one, so the
	// solver keeps preferring different days).
	gapH := math.Abs(a.start.Sub(b.start).Hours())
	p := int(float64(p0) * (8 - gapH) / 8)
	if p < 1 {
		p = 1
	}
	return p
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
		// Rooms may be shared between exams, so capacity is the aggregate booked seats of the
		// slot (a 10-student exam does not block a whole 30-seat room). Each exam's window is
		// covered per unit via allowedSlots (exahmWindowSeats), so booking length is honoured.
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

	// simulated-annealing repair on the shared optimizer core: the constructive result
	// is refined by ejecting/relocating units to free capacity and spread conflicting
	// exams. Cost, moves and construction stay pre-plan specific; only the annealing
	// loop is the generic one.
	model := &preplanModel{units: units, slots: slots, assign: assign, cost: cost, occupancy: occupancy}
	opts := optimize.DefaultOptions()
	opts.Iterations = preplanSAIterations
	opts.StartTemp = preplanSAStartTemp
	opts.EndTemp = preplanSAEndTemp
	opts.Seed = 1
	opts.StopWhenConverged = false
	optimize.Anneal(model, opts)
	return model.assign
}

// preplanModel adapts the pre-plan assignment to the generic optimize.Model interface:
// the state is the per-unit slot assignment, Cost is the pre-plan soft/drop cost, and a
// Propose is one proposeMove (relocate-with-ejection or swap), undone by restoring the
// previous assignment.
type preplanModel struct {
	units     []*preplanUnit
	slots     []*preplanSlot
	assign    []int
	cost      func([]int) float64
	occupancy func([]int) ([]int, [][]int)
}

func (m *preplanModel) Cost() float64 { return m.cost(m.assign) }
func (m *preplanModel) Snapshot() any { return append([]int(nil), m.assign...) }
func (m *preplanModel) Restore(s any) { copy(m.assign, s.([]int)) }
func (m *preplanModel) Propose(rng *rand.Rand) func() {
	before := append([]int(nil), m.assign...)
	if !proposeMove(m.assign, rng, m.units, m.slots, m.occupancy) {
		return nil
	}
	return func() { copy(m.assign, before) }
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
