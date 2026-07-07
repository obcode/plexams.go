package examplan

import (
	"sort"

	"github.com/obcode/plexams.go/plexams/optimize"
)

// Solve builds a start assignment and improves it with simulated annealing. With
// warmStart the start is the exams' current plan (Unit.StartSlot) so a re-run only
// improves and keeps churn low; otherwise it is a fresh most-constrained-first
// construction. The returned State is left at the best assignment found.
func Solve(p *Problem, opts optimize.Options, warmStart bool) (*State, optimize.Result) {
	var st *State
	if warmStart {
		st = constructWarm(p)
	} else {
		st = construct(p)
	}
	res := optimize.Anneal(st, opts)
	return st, res
}

// constructWarm starts from the exams' current plan: each movable unit is placed into
// its StartSlot if that is still hard-feasible (in a deterministic order, so a later
// unit yields to an already-placed one when a new constraint now forbids the pair). The
// remaining units (no start slot, or no longer feasible there) are filled in greedily,
// exactly like a cold construct. This keeps a re-run close to the previous plan.
func constructWarm(p *Problem) *State {
	st := newState(p)
	done := make([]bool, len(p.Units))
	for u := range p.Units {
		if p.Units[u].Fixed {
			done[u] = true
		}
	}
	for _, u := range p.movable { // deterministic order (unit index)
		if s := p.Units[u].StartSlot; s >= 0 && st.feasible(u, s) {
			st.setPhysical(u, s)
			done[u] = true
		}
	}
	fillRemaining(st, done)
	st.initCost()
	return st
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
	fillRemaining(st, done)
	st.initCost()
	return st
}

// fillRemaining greedily places every movable unit not yet marked done, most-
// constrained first (fewest feasible slots, then largest), each into the feasible slot
// that adds the least cost. Units with no feasible slot are left unplaced for the SA.
// done must already cover the fixed units (and, for a warm start, the ones kept at their
// current slot). It does not call initCost — the caller does.
func fillRemaining(st *State, done []bool) {
	p := st.P
	for {
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
		if best == -1 {
			return // all movable units handled
		}
		done[best] = true
		if len(bestFeas) == 0 {
			continue // leave for the SA repair
		}
		st.setPhysical(best, chooseSlot(st, best, bestFeas))
	}
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
	for _, v := range p.hardConfSorted[u] { // conflict partners (same slot is infeasible anyway)
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
	c += p.timePenalty(seats, s)
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
			spreadC{p.W}, attractC{p.W}, slotLoadC{p.W}, holeC{p.W}, tbauFillC{p.W}, timeOfDayC{p.W}, placementC{p.W},
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
		Description: "Zwei Prüfungen eines/einer Studierenden dürfen sich zeitlich nicht überlappen — inkl. der (ggf. per NTA verlängerten) Prüfungsdauer plus Puffer für Wechsel/Weg (außer das Paar ist als gleichzeitig erlaubt markiert)."}
}
func (sameStudentC) Check(st *State) []optimize.Violation {
	var vs []optimize.Violation
	for si := range st.P.Students {
		for _, pr := range st.P.Students[si].Pairs {
			a, b := st.SlotOf[pr.A], st.SlotOf[pr.B]
			if a >= 0 && b >= 0 && st.P.overlaps(pr.A, a, pr.B, b) {
				vs = append(vs, optimize.Violation{Constraint: "student-clash", Message: "zwei Prüfungen überlappen zeitlich (zu wenig Zwischenzeit)", Refs: []int{st.P.Units[pr.A].ID, st.P.Units[pr.B].ID}})
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
			vs = append(vs, optimize.Violation{Constraint: "capacity", Message: "Slot über Gesamt-Kapazität", Refs: st.P.slotDayRef(s)})
		}
		if st.slotExahm[s] > st.P.Slots[s].ExahmSeats {
			vs = append(vs, optimize.Violation{Constraint: "capacity", Message: "Slot über EXaHM-Kapazität", Refs: st.P.slotDayRef(s)})
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
	return optimize.Info{Name: "slot-load", Title: "Gleichmäßige Slot-Auslastung", Kind: optimize.KindSoft, Weight: c.w.SlotLoad, Tier: 30,
		Description: "Prüfungen möglichst gleichmäßig über die Slots verteilen: Abweichung von der idealen Auslastung (Anmeldungen / Slots) wird bestraft (leere und sehr volle Slots)."}
}
func (slotLoadC) Cost(st *State) (float64, []optimize.Violation) { return slotLoadCost(st) }

type holeC struct{ w Weights }

func (c holeC) Info() optimize.Info {
	return optimize.Info{Name: "slot-hole", Title: "Keine Lücke mitten am Tag", Kind: optimize.KindSoft, Weight: c.w.Hole, Tier: 31,
		Description: "Bleibt an einem Tag ein Slot frei, soll er nicht zwischen belegten Slots liegen (schlecht für die Aufsichtenplanung): freie Slots werden an den Tagesrand gedrängt, sonst mit einer kleinen Prüfung gefüllt."}
}
func (holeC) Cost(st *State) (float64, []optimize.Violation) { return holeCost(st) }

type tbauFillC struct{ w Weights }

func (c tbauFillC) Info() optimize.Info {
	return optimize.Info{Name: "tbau-fill", Title: "T-Bau-Räume ausnutzen (EXaHM/SEB)", Kind: optimize.KindSoft, Weight: c.w.TbauFill, Tier: 15,
		Description: "Phase EXaHM/SEB: die gebuchten T-Bau-Räume möglichst voll mit EXaHM/SEB-Prüfungen belegen (ungenutzte gebuchte Sitze werden bestraft)."}
}
func (tbauFillC) Cost(st *State) (float64, []optimize.Violation) { return tbauFillCost(st) }

type timeOfDayC struct{ w Weights }

func (c timeOfDayC) Info() optimize.Info {
	return optimize.Info{Name: "time-of-day", Title: "Tageszeit der Prüfungen", Kind: optimize.KindSoft, Weight: c.w.TimeOfDay, Tier: 32,
		Description: "Semesterabhängig: im Wintersemester frühe Slots (Beginn vor der Morgen-Grenze, z.B. 08:30) meiden; im Sommersemester möglichst früh beginnen — je später, desto schlechter. Je Anmeldung gewichtet, d.h. große Prüfungen werden nach vorne gezogen. Gebuchte T-Bau-Räume (Phase EXaHM/SEB) sind ausgenommen (im Sommer ganz, im Winter nur ein milder Sog Richtung späterer Beginn)."}
}
func (c timeOfDayC) Cost(st *State) (float64, []optimize.Violation) { return timeOfDayCost(st) }

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
