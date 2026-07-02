package examplan

import (
	"fmt"
	"math/rand"

	"github.com/obcode/plexams.go/plexams/optimize"
)

// State is the mutable assignment: SlotOf maps each unit to a slot index (-1 =
// unplaced). It implements optimize.Model (+ Converger + Detailer). slotSeats /
// slotExahm are maintained incrementally for O(1) capacity checks and the slot-load
// term.
type State struct {
	P         *Problem
	SlotOf    []int
	slotSeats []int
	slotExahm []int
}

func newState(p *Problem) *State {
	st := &State{
		P:         p,
		SlotOf:    make([]int, len(p.Units)),
		slotSeats: make([]int, len(p.Slots)),
		slotExahm: make([]int, len(p.Slots)),
	}
	for i := range st.SlotOf {
		st.SlotOf[i] = -1
	}
	for u := range p.Units {
		if p.Units[u].Fixed {
			st.place(u, p.Units[u].FixedSlot)
		}
	}
	return st
}

func (st *State) place(u, s int) {
	st.SlotOf[u] = s
	st.slotSeats[s] += st.P.Units[u].Seats
	if st.P.Units[u].Exahm {
		st.slotExahm[s] += st.P.Units[u].Seats
	}
}

func (st *State) unplace(u int) {
	s := st.SlotOf[u]
	if s < 0 {
		return
	}
	st.slotSeats[s] -= st.P.Units[u].Seats
	if st.P.Units[u].Exahm {
		st.slotExahm[s] -= st.P.Units[u].Seats
	}
	st.SlotOf[u] = -1
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
	return true
}

// Propose applies a random hard-feasible move (relocate 70%, swap 30%) and returns
// its undo, or nil to skip this step.
func (st *State) Propose(rng *rand.Rand) func() {
	if len(st.P.movable) == 0 || len(st.P.Slots) == 0 {
		return nil
	}
	if rng.Float64() < 0.7 || len(st.P.movable) < 2 {
		return st.relocate(rng)
	}
	return st.swap(rng)
}

func (st *State) relocate(rng *rand.Rand) func() {
	u := st.P.movable[rng.Intn(len(st.P.movable))]
	s := rng.Intn(len(st.P.Slots))
	old := st.SlotOf[u]
	if s == old {
		return nil
	}
	if old >= 0 {
		st.unplace(u)
	}
	if !st.feasible(u, s) {
		if old >= 0 {
			st.place(u, old)
		}
		return nil
	}
	st.place(u, s)
	return func() {
		st.unplace(u)
		if old >= 0 {
			st.place(u, old)
		}
	}
}

func (st *State) swap(rng *rand.Rand) func() {
	u := st.P.movable[rng.Intn(len(st.P.movable))]
	v := st.P.movable[rng.Intn(len(st.P.movable))]
	su, sv := st.SlotOf[u], st.SlotOf[v]
	if u == v || su < 0 || sv < 0 || su == sv {
		return nil
	}
	st.unplace(u)
	st.unplace(v)
	if !st.feasible(u, sv) || !st.feasible(v, su) {
		st.place(u, su)
		st.place(v, sv)
		return nil
	}
	st.place(u, sv)
	st.place(v, su)
	return func() {
		st.unplace(u)
		st.unplace(v)
		st.place(u, su)
		st.place(v, sv)
	}
}

// Cost is the total soft objective (full recompute; incremental maintenance is a
// possible later optimization).
func (st *State) Cost() float64 {
	total, _ := spreadCost(st)
	a, _ := attractCost(st)
	l, _ := slotLoadCost(st)
	u, _ := unplacedCost(st)
	return total + a + l + u
}

func (st *State) Snapshot() any {
	return snapshot{cp(st.SlotOf), cp(st.slotSeats), cp(st.slotExahm)}
}

func (st *State) Restore(a any) {
	sn := a.(snapshot)
	copy(st.SlotOf, sn.slotOf)
	copy(st.slotSeats, sn.slotSeats)
	copy(st.slotExahm, sn.slotExahm)
}

type snapshot struct{ slotOf, slotSeats, slotExahm []int }

func cp(s []int) []int {
	out := make([]int, len(s))
	copy(out, s)
	return out
}

// Converged reports whether every movable unit is placed (the schedule is complete).
func (st *State) Converged() bool {
	return st.unplacedCount() == 0
}

func (st *State) Detail() string {
	if n := st.unplacedCount(); n > 0 {
		return fmt.Sprintf("%d Prüfungen noch ungeplant", n)
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

// --- soft cost terms (shared by Model.Cost and the reporting Registry) ---

func spreadCost(st *State) (float64, []optimize.Violation) {
	p := st.P
	var total float64
	var vs []optimize.Violation
	for si := range p.Students {
		var ps float64
		for _, pr := range p.Students[si].Pairs {
			a, b := st.SlotOf[pr.A], st.SlotOf[pr.B]
			if a < 0 || b < 0 || a == b {
				continue // unplaced handled elsewhere; same slot is a hard violation
			}
			ps += pr.Weight * p.closeness(a, b)
		}
		total += ps + p.W.WorstCase*ps*ps
	}
	return total, vs
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
		over := st.slotSeats[s] - p.W.LoadThreshold
		if over > 0 {
			total += p.W.SlotLoad * float64(over) * float64(over)
		}
	}
	return total, nil
}

func unplacedCost(st *State) (float64, []optimize.Violation) {
	n := st.unplacedCount()
	var vs []optimize.Violation
	if n > 0 {
		vs = append(vs, optimize.Violation{Constraint: "placement", Message: fmt.Sprintf("%d Prüfungen ohne Slot", n)})
	}
	return st.P.W.Unplaced * float64(n), vs
}
