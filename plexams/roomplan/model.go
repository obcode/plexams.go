package roomplan

import (
	"fmt"
	"math/rand"
	"sort"
	"time"
)

// State is the mutable assignment: roomOf[i] is the room index of seat i (-1 = unplaced).
// It implements optimize.Model (+ Converger + Detailer). The soft objective is maintained
// incrementally (running totals updated per single-seat move), so Cost() is O(1). The
// per-(room, slot) and per-exam structural counters are maintained alongside; Snapshot()
// stores only roomOf and Restore() rebuilds everything from it (Restore is called once, at
// the end of annealing).
type State struct {
	P      *Problem
	roomOf []int

	// structural counters (derived from roomOf), maintained incrementally
	cellUsed  [][]int // [room][slot] seats placed (normal + alone) in this slot
	cellAlone [][]int // [room][slot] NTA-alone seats present (exclusivity; ≤1)
	examRoom  [][]int // [exam][room] seats of the exam in the room
	examRooms []int   // [exam] number of distinct rooms the exam uses
	roomAny   []int   // [room] total seats across all slots (for the distinct-room count)

	distinctRooms int // rooms with any use (compaction)

	// running soft-cost totals
	bufferByExam []float64 // [exam] the exam's current free-seat-buffer penalty
	bufferTotal  float64
	heatTotal    float64
	splitTotal   float64
	compactTotal float64
	churnTotal   float64
	prefTotal    float64 // SEB-avoid-EXaHM + own-EXaHM-fallback room preferences
	nUnplaced    int
}

func newState(p *Problem) *State {
	st := &State{
		P:            p,
		roomOf:       make([]int, len(p.Seats)),
		bufferByExam: make([]float64, len(p.Exams)),
	}
	for i := range st.roomOf {
		if p.Seats[i].Fixed {
			st.roomOf[i] = p.Seats[i].FixedRoom
		} else {
			st.roomOf[i] = -1
		}
	}
	st.rebuild()
	return st
}

// rebuild recomputes every derived counter and cost total from roomOf. Used by newState
// and Restore (both rare); the annealing loop maintains the same quantities incrementally.
func (st *State) rebuild() {
	p := st.P
	st.cellUsed = make2D(len(p.Rooms), len(p.Slots))
	st.cellAlone = make2D(len(p.Rooms), len(p.Slots))
	st.examRoom = make2D(len(p.Exams), len(p.Rooms))
	st.examRooms = make([]int, len(p.Exams))
	st.roomAny = make([]int, len(p.Rooms))
	st.distinctRooms = 0

	for i := range st.roomOf {
		if r := st.roomOf[i]; r >= 0 {
			st.structuralAdd(i, r)
		}
	}

	// cost totals
	st.nUnplaced = 0
	for _, i := range p.movable {
		if st.roomOf[i] < 0 {
			st.nUnplaced++
		}
	}
	st.heatTotal = 0
	st.churnTotal = 0
	st.prefTotal = 0
	for i := range st.roomOf {
		r := st.roomOf[i]
		st.heatTotal += p.heatCostOf(i, r)
		st.churnTotal += st.churnCostOf(i, r)
		st.prefTotal += p.sebAvoidCostOf(i, r) + p.ownExahmCostOf(i, r)
	}
	st.splitTotal = 0
	for e := range p.Exams {
		st.splitTotal += p.W.Split * float64(extraRooms(st.examRooms[e]))
	}
	st.compactTotal = p.W.Compaction * float64(st.distinctRooms)
	st.bufferTotal = 0
	for e := range p.Exams {
		st.bufferByExam[e] = st.bufferPenaltyOf(e)
		st.bufferTotal += st.bufferByExam[e]
	}
}

// structuralAdd applies seat i entering room r to the structural counters (no cost).
func (st *State) structuralAdd(i, r int) {
	p := st.P
	e := p.Seats[i].Exam
	s := p.Exams[e].Slot
	st.cellUsed[r][s]++
	if p.Seats[i].Kind == NTAAlone {
		st.cellAlone[r][s]++
	}
	if st.examRoom[e][r] == 0 {
		st.examRooms[e]++
	}
	st.examRoom[e][r]++
	if st.roomAny[r] == 0 {
		st.distinctRooms++
	}
	st.roomAny[r]++
}

// structuralRemove reverses structuralAdd for seat i leaving room r.
func (st *State) structuralRemove(i, r int) {
	p := st.P
	e := p.Seats[i].Exam
	s := p.Exams[e].Slot
	st.cellUsed[r][s]--
	if p.Seats[i].Kind == NTAAlone {
		st.cellAlone[r][s]--
	}
	st.examRoom[e][r]--
	if st.examRoom[e][r] == 0 {
		st.examRooms[e]--
	}
	st.roomAny[r]--
	if st.roomAny[r] == 0 {
		st.distinctRooms--
	}
}

// feasible reports whether seat i may occupy room r (i assumed currently NOT in r):
// room allowed for the seat kind, NTA-alone exclusivity, per-(room, slot) seat capacity
// (incl. overrun from earlier slots and this seat's own overrun into later slots), and —
// in summer for an own room — no use in a directly adjacent same-day slot (heat cooldown).
func (st *State) feasible(i, r int) bool {
	p := st.P
	if !p.allows(i, r) {
		return false
	}
	e := p.Seats[i].Exam
	s := p.Exams[e].Slot

	if p.Seats[i].Kind == NTAAlone {
		if st.cellUsed[r][s] > 0 { // needs the whole (room, slot) cell to itself
			return false
		}
	} else {
		if st.cellAlone[r][s] > 0 { // a room held alone by an NTA takes no one else
			return false
		}
		if st.cellUsed[r][s]+1 > p.Rooms[r].Seats {
			return false
		}
	}

	// room turnaround: two uses of the same room on a day must leave enough time between the
	// earlier exam's end and the later exam's start (the ordinary turnaround, widened by each
	// exam's extra Vor-/Nachlauf). This is the time-exclusivity the room-distance validation
	// enforces; concurrent exams in the SAME slot share the room freely and are exempt.
	if st.turnaroundConflict(e, r) {
		return false
	}

	// summer heat cooldown: an own room must not be used in two directly consecutive
	// same-day slots. r is unused by seat i right now, so any use of r in a neighbour
	// slot would come from another exam.
	if p.Summer && p.Rooms[r].OwnRoom {
		if prev := p.prevInDay[s]; prev >= 0 && st.cellUsed[r][prev] > 0 {
			return false
		}
		if next := p.nextInDay[s]; next >= 0 && st.cellUsed[r][next] > 0 {
			return false
		}
	}
	return true
}

// turnaroundConflict reports whether using room r for exam e would leave too little turnaround
// time to another exam using r on the same day. Concurrent exams in e's own slot share the room
// and are exempt; only different same-day slots matter. Mirrors the room-distance validation:
// between an earlier exam (end = start + duration) and a later one, the gap must be at least the
// ordinary turnaround plus the earlier exam's extra Nachlauf plus the later exam's extra Vorlauf.
func (st *State) turnaroundConflict(e, r int) bool {
	p := st.P
	s := p.Exams[e].Slot
	lag := p.timelag()
	for _, t := range p.sameDaySlots[s] {
		if st.cellUsed[r][t] == 0 {
			continue
		}
		d := int(p.Slots[t].Start.Sub(p.Slots[s].Start).Minutes())
		dur, preExtra, postExtra := st.cellUseAt(r, t)
		if d < 0 { // t is the earlier use (by -d min), e the later
			if -d < dur+lag+postExtra+p.Exams[e].PreExtra {
				return true
			}
		} else { // e is the earlier use, t the later (by d min)
			if d < p.Exams[e].Duration+lag+p.Exams[e].PostExtra+preExtra {
				return true
			}
		}
	}
	return false
}

// cellUseAt returns the max exam Duration and the max extra Vor-/Nachlauf among the exams using
// room r in slot t (several concurrent exams may share the room), mirroring the room-distance
// validation which takes the widest window at each start time.
func (st *State) cellUseAt(r, t int) (dur, preExtra, postExtra int) {
	p := st.P
	for _, e := range p.examsInSlot[t] {
		if st.examRoom[e][r] == 0 {
			continue
		}
		if p.Exams[e].Duration > dur {
			dur = p.Exams[e].Duration
		}
		if p.Exams[e].PreExtra > preExtra {
			preExtra = p.Exams[e].PreExtra
		}
		if p.Exams[e].PostExtra > postExtra {
			postExtra = p.Exams[e].PostExtra
		}
	}
	return
}

// moveSeat moves seat i to newRoom (newRoom != its current room; -1 unplaces it),
// maintaining all counters and cost totals incrementally, and returns an undo closure.
// The caller guarantees hard-feasibility of a placement (newRoom >= 0).
func (st *State) moveSeat(i, newRoom int) func() {
	p := st.P
	old := st.roomOf[i]
	e := p.Seats[i].Exam

	// save scalars + the buffer values that will change: a move only changes cellUsed in the
	// exam's own slot, so only exams sharing that slot can shift their free-seat buffer.
	affExams := p.examsInSlot[p.Exams[e].Slot]
	savedBuf := make([]float64, len(affExams))
	for k, x := range affExams {
		savedBuf[k] = st.bufferByExam[x]
	}
	saved := costTotals{st.bufferTotal, st.heatTotal, st.splitTotal, st.compactTotal, st.churnTotal, st.prefTotal, st.nUnplaced}

	beforeExtra := extraRooms(st.examRooms[e])
	deltaHeat := p.heatCostOf(i, newRoom) - p.heatCostOf(i, old)
	deltaChurn := st.churnCostOf(i, newRoom) - st.churnCostOf(i, old)
	deltaPref := (p.sebAvoidCostOf(i, newRoom) + p.ownExahmCostOf(i, newRoom)) - (p.sebAvoidCostOf(i, old) + p.ownExahmCostOf(i, old))

	if old >= 0 {
		st.structuralRemove(i, old)
	}
	if newRoom >= 0 {
		st.structuralAdd(i, newRoom)
	}
	st.roomOf[i] = newRoom

	if old < 0 && newRoom >= 0 {
		st.nUnplaced--
	} else if old >= 0 && newRoom < 0 {
		st.nUnplaced++
	}
	st.heatTotal += deltaHeat
	st.churnTotal += deltaChurn
	st.prefTotal += deltaPref
	st.splitTotal += p.W.Split * float64(extraRooms(st.examRooms[e])-beforeExtra)
	st.compactTotal = p.W.Compaction * float64(st.distinctRooms)
	for _, x := range affExams {
		st.bufferTotal -= st.bufferByExam[x]
		st.bufferByExam[x] = st.bufferPenaltyOf(x)
		st.bufferTotal += st.bufferByExam[x]
	}

	return func() {
		if newRoom >= 0 {
			st.structuralRemove(i, newRoom)
		}
		if old >= 0 {
			st.structuralAdd(i, old)
		}
		st.roomOf[i] = old
		st.bufferTotal = saved.buffer
		st.heatTotal = saved.heat
		st.splitTotal = saved.split
		st.compactTotal = saved.compact
		st.churnTotal = saved.churn
		st.prefTotal = saved.pref
		st.nUnplaced = saved.nUnplaced
		for k, x := range affExams {
			st.bufferByExam[x] = savedBuf[k]
		}
	}
}

type costTotals struct {
	buffer, heat, split, compact, churn, pref float64
	nUnplaced                                 int
}

// churnCostOf is the warm-start churn penalty for seat i sitting in room r: W.Churn when a
// previous plan is known and the seat's room differs from it. 0 when churn is disabled.
func (st *State) churnCostOf(i, r int) float64 {
	p := st.P
	if p.W.Churn == 0 || p.PrevRoom == nil {
		return 0
	}
	if prev := p.PrevRoom[i]; prev >= 0 && prev != r {
		return p.W.Churn
	}
	return 0
}

// bufferPenaltyOf is exam e's free-seat-buffer shortfall penalty: W.Buffer times how far
// the free seats across the exam's (non-alone) rooms fall below the required buffer
// (max(2, ceil(5% of the normal students))). Free seats are shared-cell aware (they count
// the seats actually left in each cell). 0 for an exam with no placed normal seats.
func (st *State) bufferPenaltyOf(e int) float64 {
	p := st.P
	if p.W.Buffer == 0 {
		return 0
	}
	s := p.Exams[e].Slot
	free, placedNormal := 0, 0
	for r := 0; r < len(p.Rooms); r++ {
		if st.examRoom[e][r] == 0 {
			continue
		}
		if st.cellAlone[r][s] > 0 { // the exam's NTA-alone room is not part of the buffer
			continue
		}
		placedNormal += st.examRoom[e][r]
		if f := p.Rooms[r].Seats - st.cellUsed[r][s]; f > 0 {
			free += f
		}
	}
	if placedNormal == 0 {
		return 0
	}
	need := freeSeatsBuffer(p.Exams[e].NormalCount)
	if free >= need {
		return 0
	}
	return p.W.Buffer * float64(need-free)
}

// Propose applies a random hard-feasible move (relocate a seat 80%, swap two seats 20%)
// and returns its undo, or nil to skip this step.
func (st *State) Propose(rng *rand.Rand) func() {
	p := st.P
	if len(p.movable) == 0 {
		return nil
	}
	if rng.Float64() < 0.8 || len(p.movable) < 2 {
		i := p.movable[rng.Intn(len(p.movable))]
		allowed := st.allowedRooms(i)
		if len(allowed) == 0 {
			return nil
		}
		r := allowed[rng.Intn(len(allowed))]
		if r == st.roomOf[i] || !st.feasible(i, r) {
			return nil
		}
		return st.moveSeat(i, r)
	}
	i := p.movable[rng.Intn(len(p.movable))]
	j := p.movable[rng.Intn(len(p.movable))]
	if i == j {
		return nil
	}
	ri, rj := st.roomOf[i], st.roomOf[j]
	if ri == rj || ri < 0 || rj < 0 {
		return nil
	}
	// remove both, then check each can take the other's room; otherwise undo.
	undoI := st.moveSeat(i, -1)
	undoJ := st.moveSeat(j, -1)
	if !st.feasible(i, rj) || !st.feasible(j, ri) {
		undoJ()
		undoI()
		return nil
	}
	placeI := st.moveSeat(i, rj)
	placeJ := st.moveSeat(j, ri)
	return func() {
		placeJ()
		placeI()
		undoJ()
		undoI()
	}
}

// allowedRooms returns the room-index list a seat may use (by its kind).
func (st *State) allowedRooms(i int) []int {
	e := st.P.Seats[i].Exam
	if st.P.Seats[i].Kind == NTAAlone {
		return st.P.Exams[e].AllowedAlone
	}
	return st.P.Exams[e].AllowedNormal
}

// Cost is the maintained total soft objective (O(1)).
func (st *State) Cost() float64 {
	return st.P.W.Unplaced*float64(st.nUnplaced) + st.bufferTotal + st.splitTotal + st.compactTotal + st.heatTotal + st.churnTotal + st.prefTotal
}

func (st *State) Snapshot() any {
	out := make([]int, len(st.roomOf))
	copy(out, st.roomOf)
	return out
}

func (st *State) Restore(a any) {
	copy(st.roomOf, a.([]int))
	st.rebuild()
}

// Converged reports whether every movable seat is placed.
func (st *State) Converged() bool { return st.nUnplaced == 0 }

func (st *State) Detail() string {
	if st.nUnplaced > 0 {
		return fmt.Sprintf("%d Sitzplätze noch ohne Raum", st.nUnplaced)
	}
	return "alle Studierenden im Raum"
}

// --- output accessors ---

// RoomAssignment is one room's use by one exam (maps to model.PlannedRoom in the caller).
type RoomAssignment struct {
	Ancode   int
	Room     string
	Start    time.Time
	Duration int
	Mtknrs   []string // students seated (sorted); empty entries (dummies) dropped
	NtaMtknr string   // set for an NTA-alone room
	Alone    bool     // NTA-alone room (HandicapRoomAlone)
}

// Assignments returns the concrete room assignments, one per (exam, room) cell, sorted by
// ancode then room name. NTA-alone seats become their own single-student Alone assignment.
func (st *State) Assignments() []RoomAssignment {
	p := st.P
	type key struct {
		exam, room int
	}
	byCell := make(map[key]*RoomAssignment)
	var order []key
	for i := range st.roomOf {
		r := st.roomOf[i]
		if r < 0 {
			continue
		}
		e := p.Seats[i].Exam
		k := key{e, r}
		a := byCell[k]
		if a == nil {
			a = &RoomAssignment{
				Ancode:   p.Exams[e].Ancode,
				Room:     p.Rooms[r].Name,
				Start:    p.Slots[p.Exams[e].Slot].Start,
				Duration: p.Exams[e].Duration,
				Alone:    p.Seats[i].Kind == NTAAlone,
			}
			byCell[k] = a
			order = append(order, k)
		}
		if p.Seats[i].Kind == NTAAlone {
			a.NtaMtknr = p.Seats[i].Mtknr
		}
		if p.Seats[i].Mtknr != "" {
			a.Mtknrs = append(a.Mtknrs, p.Seats[i].Mtknr)
		}
	}
	sort.Slice(order, func(a, b int) bool {
		ka, kb := order[a], order[b]
		if p.Exams[ka.exam].Ancode != p.Exams[kb.exam].Ancode {
			return p.Exams[ka.exam].Ancode < p.Exams[kb.exam].Ancode
		}
		return p.Rooms[ka.room].Name < p.Rooms[kb.room].Name
	})
	out := make([]RoomAssignment, 0, len(order))
	for _, k := range order {
		a := byCell[k]
		sort.Strings(a.Mtknrs)
		out = append(out, *a)
	}
	return out
}

// UnplacedSeat is one student-seat that got no room (maps to model.UnplacedExam, grouped).
type UnplacedSeat struct {
	Ancode   int
	Start    time.Time
	Mtknr    string
	NtaMtknr string
}

// Unplaced returns the seats left without a room, sorted by ancode then Mtknr.
func (st *State) Unplaced() []UnplacedSeat {
	p := st.P
	var out []UnplacedSeat
	for _, i := range p.movable {
		if st.roomOf[i] >= 0 {
			continue
		}
		e := p.Seats[i].Exam
		u := UnplacedSeat{Ancode: p.Exams[e].Ancode, Start: p.Slots[p.Exams[e].Slot].Start, Mtknr: p.Seats[i].Mtknr}
		if p.Seats[i].Kind == NTAAlone {
			u.NtaMtknr = p.Seats[i].Mtknr
		}
		out = append(out, u)
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Ancode != out[b].Ancode {
			return out[a].Ancode < out[b].Ancode
		}
		return out[a].Mtknr < out[b].Mtknr
	})
	return out
}

// UnplacedCount returns the number of movable seats without a room.
func (st *State) UnplacedCount() int { return st.nUnplaced }

// --- helpers ---

// freeSeatsBuffer mirrors roomcalc.FreeSeatsBuffer (kept inline to keep this package
// DB-agnostic): max(2, ceil(5% of the normal students)).
func freeSeatsBuffer(normalStudents int) int {
	buf := (normalStudents*5 + 99) / 100
	if buf < 2 {
		buf = 2
	}
	return buf
}

// extraRooms is the number of rooms an exam is split across beyond the first (0 when it
// uses at most one room), i.e. the penalized "split" count.
func extraRooms(nRooms int) int {
	if nRooms <= 1 {
		return 0
	}
	return nRooms - 1
}

func make2D(rows, cols int) [][]int {
	m := make([][]int, rows)
	flat := make([]int, rows*cols)
	for r := range m {
		m[r] = flat[r*cols : (r+1)*cols]
	}
	return m
}
