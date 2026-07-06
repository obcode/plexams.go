package examplan

import (
	"fmt"
	"math/rand"

	"github.com/obcode/plexams.go/plexams/optimize"
)

// State is the mutable assignment: SlotOf maps each unit to a slot index (-1 =
// unplaced). It implements optimize.Model (+ Converger + Detailer).
//
// The soft objective is maintained incrementally: slotSeats/slotExahm and the running
// cost totals (spread, attract, slot-load, unplaced) are updated per move, so Cost()
// is O(1) and a full pass over all conflict pairs is only needed once (initCost) and
// for reporting. pS[s] holds a student's current spread penalty P_s.
type State struct {
	P         *Problem
	SlotOf    []int
	slotSeats []int // total seats per slot (incl. foreign exams) — capacity & load
	slotOwn   []int // seats of OUR exams only (foreign/notPlannedByMe excluded) — used
	// for the interior-hole term: a slot holding only foreign exams is, for our
	// invigilation planning, effectively free.
	slotExahm []int
	slotSeb   []int

	pS            []float64
	spreadTotal   float64
	attractTotal  float64
	slotLoadTotal float64
	tbauFillTotal float64
	holeTotal     float64
	timeTotal     float64
	nUnplaced     int
}

func newState(p *Problem) *State {
	st := &State{
		P:         p,
		SlotOf:    make([]int, len(p.Units)),
		slotSeats: make([]int, len(p.Slots)),
		slotOwn:   make([]int, len(p.Slots)),
		slotExahm: make([]int, len(p.Slots)),
		slotSeb:   make([]int, len(p.Slots)),
		pS:        make([]float64, len(p.Students)),
	}
	for i := range st.SlotOf {
		st.SlotOf[i] = -1
	}
	for u := range p.Units {
		if p.Units[u].Fixed {
			st.setPhysical(u, p.Units[u].FixedSlot)
		}
	}
	return st
}

// setPhysical moves unit u to slot s (s = -1 unplaces it), updating only SlotOf and
// the seat arrays. Cost totals are maintained separately (moveUnit) or recomputed
// (initCost).
func (st *State) setPhysical(u, s int) {
	seats := st.P.Units[u].Seats
	exahm := st.P.Units[u].Exahm
	seb := st.P.Units[u].Seb
	own := !st.P.Units[u].Foreign
	if old := st.SlotOf[u]; old >= 0 {
		st.slotSeats[old] -= seats
		if own {
			st.slotOwn[old] -= seats
		}
		if exahm {
			st.slotExahm[old] -= seats
		}
		if seb {
			st.slotSeb[old] -= seats
		}
	}
	st.SlotOf[u] = s
	if s >= 0 {
		st.slotSeats[s] += seats
		if own {
			st.slotOwn[s] += seats
		}
		if exahm {
			st.slotExahm[s] += seats
		}
		if seb {
			st.slotSeb[s] += seats
		}
	}
}

// initCost computes the running cost totals and per-student penalties from scratch;
// call once after the constructive start, before annealing.
func (st *State) initCost() {
	p := st.P
	st.spreadTotal = 0
	for si := range p.Students {
		ps := st.studentPenalty(si)
		st.pS[si] = ps
		st.spreadTotal += ps + p.W.WorstCase*ps*ps
	}
	st.attractTotal = 0
	for _, ap := range p.Attract {
		a, b := st.SlotOf[ap.A], st.SlotOf[ap.B]
		if a >= 0 && b >= 0 {
			st.attractTotal += ap.Weight * p.farness(a, b)
		}
	}
	st.slotLoadTotal = 0
	st.tbauFillTotal = 0
	for s := range p.Slots {
		st.slotLoadTotal += p.loadPenalty(st.slotSeats[s])
		st.tbauFillTotal += p.tbauPenalty(s, st.slotExahm[s], st.slotSeb[s])
	}
	st.holeTotal = 0
	for d := range p.days {
		st.holeTotal += p.W.Hole * float64(st.dayHoleCount(d))
	}
	st.timeTotal = 0
	for u := range p.Units {
		if s := st.SlotOf[u]; s >= 0 {
			st.timeTotal += p.timePenalty(p.Units[u].Seats, s)
		}
	}
	st.nUnplaced = 0
	for _, u := range p.movable {
		if st.SlotOf[u] < 0 {
			st.nUnplaced++
		}
	}
}

// studentPenalty is P_s: the sum over the student's placed pairs of weight * closeness.
func (st *State) studentPenalty(si int) float64 {
	p := st.P
	var ps float64
	for _, pr := range p.Students[si].Pairs {
		a, b := st.SlotOf[pr.A], st.SlotOf[pr.B]
		if a < 0 || b < 0 || a == b {
			continue
		}
		c := p.closeness(a, b)
		if pr.CrossLoc && p.Slots[a].Day == p.Slots[b].Day {
			c += p.W.CrossCampus // different campuses on the same day: no travel time
		}
		ps += pr.Weight * c
	}
	return ps
}

// dayHoleCount counts the interior holes of day group d: slots without any of OUR exams
// that lie between the first and the last own-occupied slot of that day. Occupancy is
// measured in own seats (slotOwn), so a slot holding only foreign / not-planned-by-me
// exams counts as free — for our invigilation planning it is. A day whose free slots are
// all at the edges (or that is fully packed / fully empty) has 0 — good for invigilation.
func (st *State) dayHoleCount(d int) int {
	slots := st.P.days[d]
	first, last := -1, -1
	for i, s := range slots {
		if st.slotOwn[s] > 0 {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 {
		return 0
	}
	holes := 0
	for i := first + 1; i < last; i++ {
		if st.slotOwn[slots[i]] == 0 {
			holes++
		}
	}
	return holes
}

// holeOfDays is the weighted interior-hole penalty summed over the (at most two) given
// day groups; -1 entries and duplicates are ignored.
func (st *State) holeOfDays(d1, d2 int) float64 {
	if st.P.W.Hole == 0 {
		return 0
	}
	total := 0
	if d1 >= 0 {
		total += st.dayHoleCount(d1)
	}
	if d2 >= 0 && d2 != d1 {
		total += st.dayHoleCount(d2)
	}
	return st.P.W.Hole * float64(total)
}

// attractOfUnit is the attract cost contributed by unit u's attract pairs.
func (st *State) attractOfUnit(u int) float64 {
	if st.SlotOf[u] < 0 {
		return 0
	}
	p := st.P
	var c float64
	for _, ar := range p.unitAttract[u] {
		if so := st.SlotOf[ar.other]; so >= 0 {
			c += ar.weight * p.farness(st.SlotOf[u], so)
		}
	}
	return c
}

// moveUnit moves unit u to newSlot (!= its current slot), maintaining all cost totals
// incrementally, and returns an undo closure. The caller guarantees hard-feasibility.
func (st *State) moveUnit(u, newSlot int) func() {
	p := st.P
	old := st.SlotOf[u]
	affected := p.unitStudents[u]

	savedPS := make([]float64, len(affected))
	for i, s := range affected {
		savedPS[i] = st.pS[s]
	}
	savedSpread := st.spreadTotal
	savedAttract := st.attractTotal
	savedLoad := st.slotLoadTotal
	savedFill := st.tbauFillTotal
	savedHole := st.holeTotal
	savedTime := st.timeTotal
	savedUnplaced := st.nUnplaced

	// start-time avoidance delta: depends only on the moved unit's slot (and its seats)
	uSeats := p.Units[u].Seats
	st.timeTotal += p.timePenalty(uSeats, newSlot) - p.timePenalty(uSeats, old)

	// slot-load + T-building-fill deltas over the (at most two) touched slots
	loadBefore, fillBefore := 0.0, 0.0
	if old >= 0 {
		loadBefore += p.loadPenalty(st.slotSeats[old])
		fillBefore += p.tbauPenalty(old, st.slotExahm[old], st.slotSeb[old])
	}
	if newSlot >= 0 {
		loadBefore += p.loadPenalty(st.slotSeats[newSlot])
		fillBefore += p.tbauPenalty(newSlot, st.slotExahm[newSlot], st.slotSeb[newSlot])
	}
	oldAttractU := st.attractOfUnit(u)
	// interior-hole delta over the (at most two) days whose occupancy this move changes
	dOld, dNew := -1, -1
	if old >= 0 {
		dOld = p.dayOfSlot[old]
	}
	if newSlot >= 0 {
		dNew = p.dayOfSlot[newSlot]
	}
	holeBefore := st.holeOfDays(dOld, dNew)

	st.setPhysical(u, newSlot)

	loadAfter, fillAfter := 0.0, 0.0
	if old >= 0 {
		loadAfter += p.loadPenalty(st.slotSeats[old])
		fillAfter += p.tbauPenalty(old, st.slotExahm[old], st.slotSeb[old])
	}
	if newSlot >= 0 {
		loadAfter += p.loadPenalty(st.slotSeats[newSlot])
		fillAfter += p.tbauPenalty(newSlot, st.slotExahm[newSlot], st.slotSeb[newSlot])
	}
	st.slotLoadTotal += loadAfter - loadBefore
	st.tbauFillTotal += fillAfter - fillBefore
	st.holeTotal += st.holeOfDays(dOld, dNew) - holeBefore
	st.attractTotal += st.attractOfUnit(u) - oldAttractU
	if old < 0 && newSlot >= 0 {
		st.nUnplaced--
	} else if old >= 0 && newSlot < 0 {
		st.nUnplaced++
	}
	for _, s := range affected {
		st.recomputeStudentSpread(s)
	}

	return func() {
		st.setPhysical(u, old)
		for i, s := range affected {
			st.pS[s] = savedPS[i]
		}
		st.spreadTotal = savedSpread
		st.attractTotal = savedAttract
		st.slotLoadTotal = savedLoad
		st.tbauFillTotal = savedFill
		st.holeTotal = savedHole
		st.timeTotal = savedTime
		st.nUnplaced = savedUnplaced
	}
}

// recomputeStudentSpread recomputes P_s for one student and updates spreadTotal.
func (st *State) recomputeStudentSpread(si int) {
	p := st.P
	newPS := st.studentPenalty(si)
	old := st.pS[si]
	st.spreadTotal += (newPS + p.W.WorstCase*newPS*newPS) - (old + p.W.WorstCase*old*old)
	st.pS[si] = newPS
}

// feasible reports whether unit u may occupy slot s given the current assignment
// (u assumed currently NOT in s): domain, no hard-conflict unit already there, and
// EXaHM / total seat capacity.
func (st *State) feasible(u, s int) bool {
	p := st.P
	if !p.allows(u, s) {
		return false
	}
	for v := range p.hardConf[u] {
		if st.SlotOf[v] == s {
			return false
		}
	}
	seats := p.Units[u].Seats
	if cap := p.Slots[s].Seats; cap > 0 && st.slotSeats[s]+seats > cap {
		return false
	}
	// EXaHM exams may only go where enough EXaHM seats are booked; 0 booked means
	// "not an EXaHM slot" (forbidden), unlike the global seat cap where 0 = unknown.
	if p.Units[u].Exahm && st.slotExahm[s]+seats > p.Slots[s].ExahmSeats {
		return false
	}
	if !st.ntaAdjOK(u, s, -1, -1) {
		return false
	}
	return true
}

// ntaAdjOK reports whether placing unit u in slot s satisfies the NTA time-overrun
// adjacency constraints. Unit `moved` (if >= 0) is treated as sitting in `movedSlot`
// instead of its current slot — used by swaps, where the partner also changes slot.
func (st *State) ntaAdjOK(u, s, moved, movedSlot int) bool {
	p := st.P
	slotOf := func(w int) int {
		if w == moved {
			return movedSlot
		}
		return st.SlotOf[w]
	}
	if ns := p.nextSlot[s]; ns >= 0 {
		for _, b := range p.overrunNext[u] { // u overruns: no forbidden successor right after u
			if slotOf(b) == ns {
				return false
			}
		}
	}
	if ps := p.prevSlot[s]; ps >= 0 {
		for _, a := range p.overrunPrev[u] { // u sits right after an overrunning a
			if slotOf(a) == ps {
				return false
			}
		}
	}
	return true
}

// canSwap reports whether units u and v may exchange their slots without a hard
// violation (excluding each other from the conflict/capacity checks).
func (st *State) canSwap(u, v int) bool {
	p := st.P
	su, sv := st.SlotOf[u], st.SlotOf[v]
	if su < 0 || sv < 0 || su == sv {
		return false
	}
	if !p.allows(u, sv) || !p.allows(v, su) {
		return false
	}
	for w := range p.hardConf[u] {
		if w != v && st.SlotOf[w] == sv {
			return false
		}
	}
	for w := range p.hardConf[v] {
		if w != u && st.SlotOf[w] == su {
			return false
		}
	}
	su2 := st.slotSeats[sv] - p.Units[v].Seats + p.Units[u].Seats
	if cap := p.Slots[sv].Seats; cap > 0 && su2 > cap {
		return false
	}
	sv2 := st.slotSeats[su] - p.Units[u].Seats + p.Units[v].Seats
	if cap := p.Slots[su].Seats; cap > 0 && sv2 > cap {
		return false
	}
	if p.Units[u].Exahm && st.slotExahm[sv]-boolSeats(p, v)+p.Units[u].Seats > p.Slots[sv].ExahmSeats {
		return false
	}
	if p.Units[v].Exahm && st.slotExahm[su]-boolSeats(p, u)+p.Units[v].Seats > p.Slots[su].ExahmSeats {
		return false
	}
	// NTA overrun: check each unit at its post-swap slot, treating the partner as
	// already moved (so a swap that makes them adjacent is caught).
	if !st.ntaAdjOK(u, sv, v, su) || !st.ntaAdjOK(v, su, u, sv) {
		return false
	}
	return true
}

// boolSeats returns a unit's seats if it is an EXaHM exam, else 0 (its contribution
// to slotExahm).
func boolSeats(p *Problem, u int) int {
	if p.Units[u].Exahm {
		return p.Units[u].Seats
	}
	return 0
}

// Propose applies a random hard-feasible move (relocate 70%, swap 30%) and returns
// its undo, or nil to skip this step.
func (st *State) Propose(rng *rand.Rand) func() {
	if len(st.P.movable) == 0 || len(st.P.Slots) == 0 {
		return nil
	}
	if rng.Float64() < 0.7 || len(st.P.movable) < 2 {
		u := st.P.movable[rng.Intn(len(st.P.movable))]
		s := rng.Intn(len(st.P.Slots))
		if s == st.SlotOf[u] || !st.feasible(u, s) {
			return nil
		}
		return st.moveUnit(u, s)
	}
	u := st.P.movable[rng.Intn(len(st.P.movable))]
	v := st.P.movable[rng.Intn(len(st.P.movable))]
	if u == v || !st.canSwap(u, v) {
		return nil
	}
	su, sv := st.SlotOf[u], st.SlotOf[v]
	undoU := st.moveUnit(u, sv)
	undoV := st.moveUnit(v, su)
	return func() {
		undoV()
		undoU()
	}
}

// Cost is the maintained total soft objective (O(1)).
func (st *State) Cost() float64 {
	return st.spreadTotal + st.attractTotal + st.slotLoadTotal + st.tbauFillTotal + st.holeTotal + st.timeTotal + st.P.W.Unplaced*float64(st.nUnplaced)
}

func (st *State) Snapshot() any {
	return snapshot{
		slotOf: cp(st.SlotOf), slotSeats: cp(st.slotSeats), slotOwn: cp(st.slotOwn), slotExahm: cp(st.slotExahm), slotSeb: cp(st.slotSeb),
		pS: cpF(st.pS), spread: st.spreadTotal, attract: st.attractTotal, load: st.slotLoadTotal, fill: st.tbauFillTotal, hole: st.holeTotal, time: st.timeTotal, nUnplaced: st.nUnplaced,
	}
}

func (st *State) Restore(a any) {
	sn := a.(snapshot)
	copy(st.SlotOf, sn.slotOf)
	copy(st.slotSeats, sn.slotSeats)
	copy(st.slotOwn, sn.slotOwn)
	copy(st.slotExahm, sn.slotExahm)
	copy(st.slotSeb, sn.slotSeb)
	copy(st.pS, sn.pS)
	st.spreadTotal = sn.spread
	st.attractTotal = sn.attract
	st.slotLoadTotal = sn.load
	st.tbauFillTotal = sn.fill
	st.holeTotal = sn.hole
	st.timeTotal = sn.time
	st.nUnplaced = sn.nUnplaced
}

type snapshot struct {
	slotOf, slotSeats, slotOwn, slotExahm, slotSeb []int
	pS                                             []float64
	spread, attract, load, fill, hole, time        float64
	nUnplaced                                      int
}

func cp(s []int) []int {
	out := make([]int, len(s))
	copy(out, s)
	return out
}

func cpF(s []float64) []float64 {
	out := make([]float64, len(s))
	copy(out, s)
	return out
}

// Converged reports whether every movable unit is placed (the schedule is complete).
func (st *State) Converged() bool {
	return st.nUnplaced == 0
}

func (st *State) Detail() string {
	if st.nUnplaced > 0 {
		return fmt.Sprintf("%d Prüfungen noch ungeplant", st.nUnplaced)
	}
	return "alle Prüfungen geplant"
}

func (st *State) unplacedCount() int {
	n := 0
	for _, u := range st.P.movable {
		if st.SlotOf[u] < 0 {
			n++
		}
	}
	return n
}

// --- full-recompute soft cost terms (for the reporting Registry; the annealing loop
// uses the incrementally maintained totals above) ---

func spreadCost(st *State) (float64, []optimize.Violation) {
	p := st.P
	var total float64
	for si := range p.Students {
		ps := st.studentPenalty(si)
		total += ps + p.W.WorstCase*ps*ps
	}
	return total, nil
}

func attractCost(st *State) (float64, []optimize.Violation) {
	p := st.P
	var total float64
	for _, ap := range p.Attract {
		a, b := st.SlotOf[ap.A], st.SlotOf[ap.B]
		if a < 0 || b < 0 {
			continue
		}
		total += ap.Weight * p.farness(a, b)
	}
	return total, nil
}

func slotLoadCost(st *State) (float64, []optimize.Violation) {
	p := st.P
	var total float64
	for s := range p.Slots {
		total += p.loadPenalty(st.slotSeats[s])
	}
	return total, nil
}

// TbauUsage reports how many of the booked T-building EXaHM/SEB seats are used (capped
// at the booking; SEB overflow to R-rooms does not count).
func (st *State) TbauUsage() (bookedExahm, usedExahm, bookedSeb, usedSeb int) {
	for s := range st.P.Slots {
		be, bs := st.P.Slots[s].ExahmSeats, st.P.Slots[s].SebSeats
		bookedExahm += be
		bookedSeb += bs
		usedExahm += minInt(st.slotExahm[s], be)
		usedSeb += minInt(st.slotSeb[s], bs)
	}
	return
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func tbauFillCost(st *State) (float64, []optimize.Violation) {
	p := st.P
	var total float64
	for s := range p.Slots {
		total += p.tbauPenalty(s, st.slotExahm[s], st.slotSeb[s])
	}
	return total, nil
}

// holeCost sums the interior-hole penalty over all days and reports each offending slot
// (an empty slot with occupied slots both before and after it on the same day).
func holeCost(st *State) (float64, []optimize.Violation) {
	p := st.P
	var total float64
	var vs []optimize.Violation
	for d := range p.days {
		slots := p.days[d]
		first, last := -1, -1
		for i, s := range slots {
			if st.slotOwn[s] > 0 {
				if first < 0 {
					first = i
				}
				last = i
			}
		}
		if first < 0 {
			continue
		}
		for i := first + 1; i < last; i++ {
			if st.slotOwn[slots[i]] == 0 {
				total += p.W.Hole
				vs = append(vs, optimize.Violation{Constraint: "slot-hole", Message: "freier Slot (ohne eigene Prüfung) zwischen belegten Slots",
					Refs: []int{p.Slots[slots[i]].Day, p.Slots[slots[i]].Slot}})
			}
		}
	}
	return total, vs
}

// timeOfDayCost sums the start-time avoidance penalty over all placed units and reports
// each unit sitting in a penalized slot (start time outside the wanted window).
func timeOfDayCost(st *State) (float64, []optimize.Violation) {
	p := st.P
	if p.W.TimeOfDay == 0 {
		return 0, nil
	}
	var total float64
	var vs []optimize.Violation
	for u := range p.Units {
		s := st.SlotOf[u]
		if s < 0 {
			continue
		}
		c := p.timePenalty(p.Units[u].Seats, s)
		if c > 0 {
			total += c
			vs = append(vs, optimize.Violation{Constraint: "time-of-day", Message: "Prüfung in ungünstiger Tageszeit",
				Refs: []int{p.Slots[s].Day, p.Slots[s].Slot}})
		}
	}
	return total, vs
}

func unplacedCost(st *State) (float64, []optimize.Violation) {
	n := st.unplacedCount()
	var vs []optimize.Violation
	if n > 0 {
		vs = append(vs, optimize.Violation{Constraint: "placement", Message: fmt.Sprintf("%d Prüfungen ohne Slot", n)})
	}
	return st.P.W.Unplaced * float64(n), vs
}
