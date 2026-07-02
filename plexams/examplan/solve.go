package examplan

import (
	"sort"

	"github.com/obcode/plexams.go/plexams/optimize"
)

// Solve builds a constructive (most-constrained-first) start and improves it with
// simulated annealing. The returned State is left at the best assignment found.
func Solve(p *Problem, opts optimize.Options) (*State, optimize.Result) {
	st := construct(p)
	res := optimize.Anneal(st, opts)
	return st, res
}

// construct greedily places the movable units, most-constrained first (fewest
// feasible slots, then largest), each into the feasible slot that adds the least
// spread/attract/load cost. Units with no feasible slot are left unplaced for the SA.
func construct(p *Problem) *State {
	st := newState(p)
	done := make([]bool, len(p.Units))
	for u := range p.Units {
		if p.Units[u].Fixed {
			done[u] = true
		}
	}

	for remaining := len(p.movable); remaining > 0; remaining-- {
		best, bestFeas := -1, []int(nil)
		for _, u := range p.movable {
			if done[u] {
				continue
			}
			feas := feasibleSlots(st, u)
			if best == -1 || moreConstrained(p, u, len(feas), best, len(bestFeas)) {
				best, bestFeas = u, feas
			}
		}
		done[best] = true
		if len(bestFeas) == 0 {
			continue // leave for the SA repair
		}
		st.setPhysical(best, chooseSlot(st, best, bestFeas))
	}
	st.initCost()
	return st
}

func feasibleSlots(st *State, u int) []int {
	feas := make([]int, 0, len(st.P.Slots))
	for s := range st.P.Slots {
		if st.SlotOf[u] == s {
			continue
		}
		if st.feasible(u, s) {
			feas = append(feas, s)
		}
	}
	return feas
}

// moreConstrained reports whether unit u (with fu feasible slots) should be placed
// before the current best (with fb feasible slots): fewer options first, then larger
// (more seats), then smaller id.
func moreConstrained(p *Problem, u, fu, best, fb int) bool {
	if fu != fb {
		return fu < fb
	}
	if p.Units[u].Seats != p.Units[best].Seats {
		return p.Units[u].Seats > p.Units[best].Seats
	}
	return p.Units[u].ID < p.Units[best].ID
}

// chooseSlot picks the feasible slot that adds the least cost, tie-broken by most
// free capacity (lowest current seat load).
func chooseSlot(st *State, u int, feas []int) int {
	best, bestCost := feas[0], 0.0
	first := true
	for _, s := range feas {
		c := addedCost(st, u, s)
		if first || c < bestCost || (c == bestCost && st.slotSeats[s] < st.slotSeats[best]) {
			best, bestCost, first = s, c, false
		}
	}
	return best
}

// addedCost is the cost of placing unit u in slot s given the current assignment:
// spread against already-placed conflict partners, attract farness, and the marginal
// slot-load penalty.
func addedCost(st *State, u, s int) float64 {
	p := st.P
	var c float64
	for v := range p.hardConf[u] { // conflict partners (same slot is infeasible anyway)
		if sv := st.SlotOf[v]; sv >= 0 && sv != s {
			c += p.closeness(s, sv) // approx: unit-level, unweighted by student count
		}
	}
	for _, ap := range p.Attract {
		var other int
		switch {
		case ap.A == u:
			other = ap.B
		case ap.B == u:
			other = ap.A
		default:
			continue
		}
		if so := st.SlotOf[other]; so >= 0 {
			c += ap.Weight * p.farness(s, so)
		}
	}
	seats := p.Units[u].Seats
	over := st.slotSeats[s] + seats - p.W.LoadThreshold
	if over > 0 {
		c += p.W.SlotLoad * float64(over) * float64(over)
	}
	return c
}

// Registry returns the self-describing hard/soft constraints for reporting and the
// read-only "which constraints are applied" view.
func (p *Problem) Registry() optimize.Registry[*State] {
	return optimize.Registry[*State]{
		Hard: []optimize.HardConstraint[*State]{
			fixedC{}, allowedC{}, sameStudentC{}, capacityC{},
		},
		Soft: []optimize.SoftConstraint[*State]{
			spreadC{p.W}, attractC{p.W}, slotLoadC{p.W}, placementC{p.W},
		},
	}
}

type fixedC struct{}

func (fixedC) Info() optimize.Info {
	return optimize.Info{Name: "fixed", Title: "Feste Termine unverändert", Kind: optimize.KindHard, Tier: 1,
		Description: "Gesperrte, externe und nicht von mir geplante Prüfungen behalten ihren Slot."}
}
func (fixedC) Check(st *State) []optimize.Violation {
	var vs []optimize.Violation
	for u := range st.P.Units {
		if st.P.Units[u].Fixed && st.SlotOf[u] != st.P.Units[u].FixedSlot {
			vs = append(vs, optimize.Violation{Constraint: "fixed", Message: "feste Prüfung verschoben", Refs: st.P.Units[u].Ancodes})
		}
	}
	return vs
}

type allowedC struct{}

func (allowedC) Info() optimize.Info {
	return optimize.Info{Name: "allowed-slots", Title: "Zulässige Slots", Kind: optimize.KindHard, Tier: 2,
		Description: "Jede Prüfung nur in einem für sie erlaubten Slot (Constraints, gesperrte Tage, MUC.DAI)."}
}
func (allowedC) Check(st *State) []optimize.Violation {
	var vs []optimize.Violation
	for u := range st.P.Units {
		if s := st.SlotOf[u]; s >= 0 && !st.P.allows(u, s) {
			vs = append(vs, optimize.Violation{Constraint: "allowed-slots", Message: "Prüfung in unzulässigem Slot", Refs: st.P.Units[u].Ancodes})
		}
	}
	return vs
}

type sameStudentC struct{}

func (sameStudentC) Info() optimize.Info {
	return optimize.Info{Name: "student-clash", Title: "Kein Studierender doppelt", Kind: optimize.KindHard, Tier: 3,
		Description: "Kein Studierender hat zwei seiner Prüfungen im selben Slot (außer das Paar ist als gleichzeitig erlaubt markiert)."}
}
func (sameStudentC) Check(st *State) []optimize.Violation {
	var vs []optimize.Violation
	for si := range st.P.Students {
		for _, pr := range st.P.Students[si].Pairs {
			a, b := st.SlotOf[pr.A], st.SlotOf[pr.B]
			if a >= 0 && a == b {
				vs = append(vs, optimize.Violation{Constraint: "student-clash", Message: "zwei Prüfungen gleichzeitig", Refs: []int{st.P.Units[pr.A].ID, st.P.Units[pr.B].ID}})
			}
		}
	}
	return vs
}

type capacityC struct{}

func (capacityC) Info() optimize.Info {
	return optimize.Info{Name: "capacity", Title: "Slot-Kapazität", Kind: optimize.KindHard, Tier: 4,
		Description: "Pro Slot: EXaHM-Prüfungen ≤ gebuchte EXaHM-Sitze, alle Prüfungen ≤ Gesamt-Raumkapazität."}
}
func (capacityC) Check(st *State) []optimize.Violation {
	var vs []optimize.Violation
	for s := range st.P.Slots {
		if cap := st.P.Slots[s].Seats; cap > 0 && st.slotSeats[s] > cap {
			vs = append(vs, optimize.Violation{Constraint: "capacity", Message: "Slot über Gesamt-Kapazität", Refs: []int{st.P.Slots[s].Day, st.P.Slots[s].Slot}})
		}
		if st.slotExahm[s] > st.P.Slots[s].ExahmSeats {
			vs = append(vs, optimize.Violation{Constraint: "capacity", Message: "Slot über EXaHM-Kapazität", Refs: []int{st.P.Slots[s].Day, st.P.Slots[s].Slot}})
		}
	}
	return vs
}

type spreadC struct{ w Weights }

func (c spreadC) Info() optimize.Info {
	return optimize.Info{Name: "spread", Title: "Spreizung der Prüfungen", Kind: optimize.KindSoft, Weight: c.w.Adjacent, Tier: 10,
		Description: "Prüfungen eines Studierenden möglichst weit auseinander (Summe + konvexer Worst-Case je Studierendem); direkt nacheinander/selber Tag kostet am meisten, Wiederholungen mit Rabatt."}
}
func (spreadC) Cost(st *State) (float64, []optimize.Violation) { return spreadCost(st) }

type attractC struct{ w Weights }

func (c attractC) Info() optimize.Info {
	return optimize.Info{Name: "attract", Title: "Zusammen legen", Kind: optimize.KindSoft, Weight: c.w.Attract, Tier: 20,
		Description: "Parallelsektionen desselben Moduls und kleine Prüfungen desselben Prüfers möglichst in denselben Slot."}
}
func (attractC) Cost(st *State) (float64, []optimize.Violation) { return attractCost(st) }

type slotLoadC struct{ w Weights }

func (c slotLoadC) Info() optimize.Info {
	return optimize.Info{Name: "slot-load", Title: "Slot-Auslastung", Kind: optimize.KindSoft, Weight: c.w.SlotLoad, Tier: 30,
		Description: "Nicht zu viele Sitzplätze/Räume je Slot (zwei sehr große Prüfungen nicht zusammen)."}
}
func (slotLoadC) Cost(st *State) (float64, []optimize.Violation) { return slotLoadCost(st) }

type placementC struct{ w Weights }

func (c placementC) Info() optimize.Info {
	return optimize.Info{Name: "placement", Title: "Alle Prüfungen geplant", Kind: optimize.KindSoft, Weight: c.w.Unplaced, Tier: 0,
		Description: "Jede zu planende Prüfung muss einen Slot bekommen (dominante Strafe für ungeplante)."}
}
func (placementC) Cost(st *State) (float64, []optimize.Violation) { return unplacedCost(st) }

// UnplacedAncodes returns the ancodes of units left without a slot (for reporting).
func (st *State) UnplacedAncodes() []int {
	var out []int
	for _, u := range st.P.movable {
		if st.SlotOf[u] < 0 {
			out = append(out, st.P.Units[u].ID)
		}
	}
	sort.Ints(out)
	return out
}
