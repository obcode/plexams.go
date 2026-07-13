package roomplan

import (
	"fmt"
	"sort"

	"github.com/obcode/plexams.go/plexams/optimize"
)

// Solve builds a hard-feasible start assignment and improves it with simulated annealing.
// With warmStart the start is the exams' current room plan (Problem.PrevRoom) so a re-run
// only refines and keeps churn low; otherwise it is a fresh greedy construction. The
// returned State is left at the best assignment found.
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

// construct greedily fills every movable seat, larger exams first, each into the best
// feasible room (a room the exam already uses, else an already-open room, else the largest
// fitting one). Seats with no feasible room are left unplaced for the SA to repair.
func construct(p *Problem) *State {
	st := newState(p)
	fillRemaining(st)
	return st
}

// constructWarm starts from the saved plan: each movable seat is placed into its previous
// room where that is still hard-feasible (deterministic seat order, so a later seat yields
// to an already-placed one), then the rest is filled greedily like a cold construct.
func constructWarm(p *Problem) *State {
	st := newState(p)
	if p.PrevRoom != nil {
		for _, i := range p.movable {
			if r := p.PrevRoom[i]; r >= 0 && st.feasible(i, r) {
				st.moveSeat(i, r)
			}
		}
	}
	fillRemaining(st)
	return st
}

// fillRemaining places every still-unplaced movable seat into the best feasible room. Seats
// are filled slot by slot and, WITHIN a slot, round-robin across the exams (the k-th seat of
// each exam before any exam's (k+1)-th). Combined with best-fit room choice this co-packs
// exams that share a scarce room pool (e.g. two EXaHM exams into the booked T-building rooms)
// instead of letting the first exam fill whole rooms and starve the next.
func fillRemaining(st *State) {
	p := st.P
	rank := make([]int, len(p.Seats)) // seat's position within its exam (for round-robin)
	for e := range p.seatsOfExam {
		for k, i := range p.seatsOfExam[e] {
			rank[i] = k
		}
	}
	order := make([]int, len(p.movable))
	copy(order, p.movable)
	sort.SliceStable(order, func(a, b int) bool {
		ia, ib := order[a], order[b]
		ea, eb := p.Seats[ia].Exam, p.Seats[ib].Exam
		if p.Exams[ea].Slot != p.Exams[eb].Slot {
			return p.Exams[ea].Slot < p.Exams[eb].Slot
		}
		if rank[ia] != rank[ib] { // round-robin: one seat from each exam, then the next
			return rank[ia] < rank[ib]
		}
		return ia < ib
	})
	for _, i := range order {
		if st.roomOf[i] >= 0 {
			continue
		}
		if r := st.chooseRoom(i); r >= 0 {
			st.moveSeat(i, r)
		}
	}
}

// chooseRoom returns the best feasible room for the still-unplaced seat i, or -1: best-fit —
// the fullest room that still has space (packs shared rooms tight before opening a new one),
// then, among equally-full (e.g. still-empty) rooms, the largest; ties broken by room index.
func (st *State) chooseRoom(i int) int {
	p := st.P
	best, bestScore := -1, [2]int{}
	for _, r := range st.allowedRooms(i) {
		if !st.feasible(i, r) {
			continue
		}
		score := [2]int{st.cellUsed[r][p.Exams[p.Seats[i].Exam].Slot], p.Rooms[r].Seats}
		if best < 0 || less2(bestScore, score) {
			best, bestScore = r, score
		}
	}
	return best
}

// Registry returns the self-describing hard/soft constraints for reporting and the
// read-only "which constraints are applied" view. Hard constraints are enforced inside
// Propose; their Check re-validates a finished state.
func (p *Problem) Registry() optimize.Registry[*State] {
	return optimize.Registry[*State]{
		Hard: []optimize.HardConstraint[*State]{
			allowedRoomC{}, capacityC{}, ntaAloneC{}, prePlannedC{}, overrunC{}, summerCooldownC{}, exahmWindowC{},
		},
		Soft: []optimize.SoftConstraint[*State]{
			placementC{}, bufferC{}, splitC{}, compactionC{}, sebRbauC{}, exahmBookedC{}, heatFloorC{}, churnC{},
		},
	}
}

// --- hard constraints ---

type allowedRoomC struct{}

func (allowedRoomC) Info() optimize.Info {
	return optimize.Info{Name: "allowed-room", Title: "Raum passt & ist verfügbar", Kind: optimize.KindHard, Tier: 1,
		Description: "Jede Prüfung nur in Räumen, die zu ihr passen und in ihrem Slot verfügbar sind: EXaHM/SEB/Lab/Steckdose/erlaubte-Räume erfüllt, nicht gesperrt/deaktiviert, und Handicap-Räume nur für NTA."}
}
func (allowedRoomC) Check(st *State) []optimize.Violation {
	var vs []optimize.Violation
	for i := range st.roomOf {
		if r := st.roomOf[i]; r >= 0 && !st.P.allows(i, r) {
			e := st.P.Seats[i].Exam
			vs = append(vs, optimize.Violation{Constraint: "allowed-room", Message: "Sitz in unzulässigem Raum (" + st.P.Rooms[r].Name + ")", Refs: []int{st.P.Exams[e].Ancode}})
		}
	}
	return vs
}

type capacityC struct{}

func (capacityC) Info() optimize.Info {
	return optimize.Info{Name: "capacity", Title: "Raumkapazität pro Slot", Kind: optimize.KindHard, Tier: 2,
		Description: "Belegte Sitze je (Raum, Slot) – inkl. der aus Nachbarslots hereinlaufenden Nachlauf-Belegung – dürfen die Sitzplätze des Raums nicht übersteigen. Räume werden von mehreren Prüfungen geteilt."}
}
func (capacityC) Check(st *State) []optimize.Violation {
	var vs []optimize.Violation
	for r := range st.P.Rooms {
		for s := range st.P.Slots {
			if st.cellUsed[r][s] > st.P.Rooms[r].Seats {
				vs = append(vs, optimize.Violation{Constraint: "capacity",
					Message: fmt.Sprintf("Raum %s zum Slot %s überbelegt", st.P.Rooms[r].Name, st.P.Slots[s].Start.Format("02.01. 15:04"))})
			}
		}
	}
	return vs
}

type ntaAloneC struct{}

func (ntaAloneC) Info() optimize.Info {
	return optimize.Info{Name: "nta-alone", Title: "NTA mit eigenem Raum wirklich allein", Kind: optimize.KindHard, Tier: 3,
		Description: "Ein NTA, der einen Raum allein braucht, belegt seine (Raum, Slot)-Zelle exklusiv – niemand sonst und keine Nachlauf-Belegung im selben Raum."}
}
func (ntaAloneC) Check(st *State) []optimize.Violation {
	var vs []optimize.Violation
	for r := range st.P.Rooms {
		for s := range st.P.Slots {
			if a := st.cellAlone[r][s]; a > 0 {
				if a > 1 || st.cellUsed[r][s] != a {
					vs = append(vs, optimize.Violation{Constraint: "nta-alone",
						Message: fmt.Sprintf("NTA-Alleinraum %s zum Slot %s nicht exklusiv", st.P.Rooms[r].Name, st.P.Slots[s].Start.Format("02.01. 15:04"))})
				}
			}
		}
	}
	return vs
}

type prePlannedC struct{}

func (prePlannedC) Info() optimize.Info {
	return optimize.Info{Name: "pre-planned", Title: "Vorbelegte Räume eingehalten", Kind: optimize.KindHard, Tier: 4,
		Description: "Manuell vorbelegte Räume (inkl. exakter Sitzvorgaben) bleiben fix und werden vom Solver nicht verändert."}
}
func (prePlannedC) Check(st *State) []optimize.Violation {
	var vs []optimize.Violation
	for i := range st.P.Seats {
		if s := &st.P.Seats[i]; s.Fixed && st.roomOf[i] != s.FixedRoom {
			vs = append(vs, optimize.Violation{Constraint: "pre-planned", Message: "vorbelegter Sitz verschoben", Refs: []int{st.P.Exams[s.Exam].Ancode}})
		}
	}
	return vs
}

type overrunC struct{}

func (overrunC) Info() optimize.Info {
	return optimize.Info{Name: "room-overrun", Title: "Raum-Umbauzeit zwischen zwei Nutzungen", Kind: optimize.KindHard, Tier: 5,
		Description: "Zwischen zwei Prüfungen im selben Raum an einem Tag muss genug Umbauzeit liegen (übliche Umbauzeit plus zusätzlicher Vor-/Nachlauf, z. B. bei EXaHM). Eine lange Prüfung belegt ihren Raum bis in Nachbarslots hinein."}
}
func (overrunC) Check(st *State) []optimize.Violation {
	p := st.P
	lag := p.timelag()
	var vs []optimize.Violation
	for r := range p.Rooms {
		// gather the slots room r is used in, chronologically, with their widest window.
		var used []int
		for s := range p.Slots {
			if st.cellUsed[r][s] > 0 {
				used = append(used, s)
			}
		}
		sort.Slice(used, func(a, b int) bool { return p.Slots[used[a]].Start.Before(p.Slots[used[b]].Start) })
		for k := 1; k < len(used); k++ {
			prev, cur := used[k-1], used[k]
			if p.dayOfSlot[prev] != p.dayOfSlot[cur] {
				continue
			}
			dur, _, prevPost := st.cellUseAt(r, prev)
			_, curPre, _ := st.cellUseAt(r, cur)
			gap := int(p.Slots[cur].Start.Sub(p.Slots[prev].Start).Minutes())
			if gap < dur+lag+prevPost+curPre {
				vs = append(vs, optimize.Violation{Constraint: "room-overrun",
					Message: fmt.Sprintf("zu wenig Umbauzeit in Raum %s zwischen %s und %s",
						p.Rooms[r].Name, p.Slots[prev].Start.Format("02.01. 15:04"), p.Slots[cur].Start.Format("15:04"))})
			}
		}
	}
	return vs
}

type summerCooldownC struct{}

func (summerCooldownC) Info() optimize.Info {
	return optimize.Info{Name: "summer-cooldown", Title: "Sommer: eigener Raum nicht zweimal hintereinander", Kind: optimize.KindHard, Tier: 6,
		Description: "Hitzeschutz: im Sommer darf ein eigener (nicht gebuchter) Raum nicht in zwei direkt aufeinanderfolgenden Slots eines Tages genutzt werden – Zeit zum Abkühlen/Lüften. Gebuchte Räume sind ausgenommen."}
}
func (summerCooldownC) Check(st *State) []optimize.Violation {
	p := st.P
	if !p.Summer {
		return nil
	}
	var vs []optimize.Violation
	for r := range p.Rooms {
		if !p.Rooms[r].OwnRoom {
			continue
		}
		for s := range p.Slots {
			if st.cellUsed[r][s] == 0 {
				continue
			}
			if next := p.nextInDay[s]; next >= 0 && st.cellUsed[r][next] > 0 {
				vs = append(vs, optimize.Violation{Constraint: "summer-cooldown",
					Message: fmt.Sprintf("Raum %s in zwei aufeinanderfolgenden Slots (%s)", p.Rooms[r].Name, p.Slots[s].Start.Format("02.01. 15:04"))})
			}
		}
	}
	return vs
}

type exahmWindowC struct{}

func (exahmWindowC) Info() optimize.Info {
	return optimize.Info{Name: "exahm-window", Title: "EXaHM/SEB nur in gebuchten T-Bau-Räumen", Kind: optimize.KindHard, Tier: 7,
		Description: "EXaHM/SEB-Prüfungen liegen nur in gebuchten T-Bau-Räumen, deren Anny-Buchung das Prüfungsfenster (Dauer + Vor-/Nachlauf) abdeckt. EXaHM braucht EXaHM-Räume; SEB akzeptiert EXaHM- oder SEB-Räume. (Über die erlaubten Räume je Slot erzwungen.)"}
}
func (exahmWindowC) Check(st *State) []optimize.Violation {
	p := st.P
	var vs []optimize.Violation
	for i := range st.roomOf {
		r := st.roomOf[i]
		if r < 0 {
			continue
		}
		e := p.Seats[i].Exam
		room := &p.Rooms[r]
		if p.Exams[e].Exahm && !room.Exahm {
			vs = append(vs, optimize.Violation{Constraint: "exahm-window", Message: "EXaHM-Prüfung nicht in EXaHM-Raum", Refs: []int{p.Exams[e].Ancode}})
		} else if p.Exams[e].Seb && !room.Exahm && !room.Seb {
			vs = append(vs, optimize.Violation{Constraint: "exahm-window", Message: "SEB-Prüfung nicht in SEB/EXaHM-Raum", Refs: []int{p.Exams[e].Ancode}})
		}
	}
	return vs
}

// --- soft constraints ---

type placementC struct{}

func (placementC) Info() optimize.Info {
	return optimize.Info{Name: "placement", Title: "Alle Studierenden bekommen einen Sitzplatz", Kind: optimize.KindSoft, Tier: 0,
		Description: "Jeder Sitzplatz muss einen realen Raum bekommen (dominante Strafe für ungeplante Sitze)."}
}
func (placementC) Cost(st *State) (float64, []optimize.Violation) {
	n := st.UnplacedCount()
	var vs []optimize.Violation
	if n > 0 {
		vs = append(vs, optimize.Violation{Constraint: "placement", Message: fmt.Sprintf("%d Sitzplätze ohne Raum", n)})
	}
	return st.P.W.Unplaced * float64(n), vs
}

type bufferC struct{}

func (bufferC) Info() optimize.Info {
	return optimize.Info{Name: "free-buffer", Title: "Freisitz-Puffer je Prüfung", Kind: optimize.KindSoft, Tier: 20,
		Description: "Jede Prüfung soll über ihre Räume einen Puffer freier Plätze behalten (mind. 2 bzw. 5 % der Anmeldungen) – kein Raum wird randvoll gepackt. Der Puffer entsteht faktisch als teilbelegter Reserve-Raum."}
}
func (bufferC) Cost(st *State) (float64, []optimize.Violation) {
	var total float64
	for e := range st.P.Exams {
		total += st.bufferPenaltyOf(e)
	}
	return total, nil
}

type splitC struct{}

func (splitC) Info() optimize.Info {
	return optimize.Info{Name: "exam-split", Title: "Prüfung zusammenhalten", Kind: optimize.KindSoft, Tier: 21,
		Description: "Eine Prüfung soll auf möglichst wenige Räume verteilt werden; jeder zusätzliche Raum wird bestraft (Kapazität erzwingt große Prüfungen dennoch in mehrere Räume)."}
}
func (splitC) Cost(st *State) (float64, []optimize.Violation) {
	var total float64
	for e := range st.P.Exams {
		total += st.P.W.Split * float64(extraRooms(st.examRooms[e]))
	}
	return total, nil
}

type compactionC struct{}

func (compactionC) Info() optimize.Info {
	return optimize.Info{Name: "room-compaction", Title: "Wenige verschiedene Räume gesamt", Kind: optimize.KindSoft, Tier: 22,
		Description: "Über den ganzen Plan möglichst wenige verschiedene Räume nutzen, damit weniger Räume angefragt/geöffnet werden müssen."}
}
func (compactionC) Cost(st *State) (float64, []optimize.Violation) {
	return st.P.W.Compaction * float64(st.distinctRooms), nil
}

type sebRbauC struct{}

func (sebRbauC) Info() optimize.Info {
	return optimize.Info{Name: "seb-rbau", Title: "SEB möglichst im R-Bau", Kind: optimize.KindSoft, Tier: 15,
		Description: "SEB-Prüfungen bevorzugt in normale SEB-Räume (R-Bau) legen und die knappen gebuchten T-Bau-EXaHM-Räume für echte EXaHM-Prüfungen frei halten. Reichen die R-Bau-SEB-Räume nicht, darf SEB weiter in den T-Bau."}
}
func (sebRbauC) Cost(st *State) (float64, []optimize.Violation) {
	var total float64
	for i := range st.roomOf {
		total += st.P.sebAvoidCostOf(i, st.roomOf[i])
	}
	return total, nil
}

type exahmBookedC struct{}

func (exahmBookedC) Info() optimize.Info {
	return optimize.Info{Name: "exahm-booked", Title: "EXaHM bevorzugt in gebuchten T-Bau-Räumen", Kind: optimize.KindSoft, Tier: 16,
		Description: "EXaHM-Prüfungen (inkl. NTA-Alleinräume) bevorzugt in die gebuchten T-Bau-Räume; ein eigener EXaHM-Raum im R-Bau (z. B. der 1-Platz-NTA-Raum R1.011) wird nur als Ausweichlösung genutzt, wenn kein gebuchter T-Bau-Raum verfügbar ist (etwa der NTA-Raum T3.021)."}
}
func (exahmBookedC) Cost(st *State) (float64, []optimize.Violation) {
	var total float64
	for i := range st.roomOf {
		total += st.P.ownExahmCostOf(i, st.roomOf[i])
	}
	return total, nil
}

type heatFloorC struct{}

func (heatFloorC) Info() optimize.Info {
	return optimize.Info{Name: "heat-floor", Title: "Sommer: spät = weiter unten", Kind: optimize.KindSoft, Tier: 23,
		Description: "Hitzeschutz: je später am Tag eine Prüfung liegt, desto tiefer soll ihr Stockwerk sein. Bestraft wird Stockwerk × Tageszeit je Sitz in eigenen Räumen; gebuchte Räume sind ausgenommen. Nur im Sommer aktiv."}
}
func (heatFloorC) Cost(st *State) (float64, []optimize.Violation) {
	var total float64
	for i := range st.roomOf {
		total += st.P.heatCostOf(i, st.roomOf[i])
	}
	return total, nil
}

type churnC struct{}

func (churnC) Info() optimize.Info {
	return optimize.Info{Name: "churn", Title: "Wenig Änderung bei Neu-Lauf", Kind: optimize.KindSoft, Tier: 24,
		Description: "Bei einem erneuten Lauf möglichst nah am gespeicherten Raumplan bleiben (Warm-Start); jeder gegenüber vorher geänderte Sitz wird leicht bestraft."}
}
func (churnC) Cost(st *State) (float64, []optimize.Violation) {
	var total float64
	for i := range st.roomOf {
		total += st.churnCostOf(i, st.roomOf[i])
	}
	return total, nil
}

// --- helpers ---

// less2 orders the chooseRoom score tuples (higher is better in each component).
func less2(a, b [2]int) bool {
	for k := 0; k < 2; k++ {
		if a[k] != b[k] {
			return a[k] < b[k]
		}
	}
	return false
}
