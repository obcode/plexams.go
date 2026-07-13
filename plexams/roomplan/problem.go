// Package roomplan assigns concrete rooms to the exams of an already-fixed exam
// schedule (Terminplan): every exam runs at a known absolute time; this solver only
// decides which room(s) seat its students. It is built on the generic simulated-
// annealing core in plexams/optimize and is deliberately DB-agnostic — the caller
// (plexams package) builds a *Problem from the domain data (assembled/planned exams,
// rooms, pre-planned rooms, room availability per slot) and maps the resulting seat →
// room assignment back to model.PlannedRoom / model.UnplacedExam.
//
// The decision variable is the room of a single student-seat (roomOf[i]); exam splits
// across several rooms and shared rooms within a slot therefore emerge naturally from
// single-seat moves, with no per-room count bookkeeping. Rooms are shared within a slot
// (a 10-seat exam does not block a 30-seat room); an exam may split across rooms; an
// NTA needing a room alone occupies its (room, slot) cell exclusively.
//
// Hard constraints (enforced inside Propose, re-validated for reporting): a seat only
// in a room allowed for it (feature/availability/handicap pre-filtered into AllowedX),
// per-(room, slot) seat capacity, NTA-alone exclusivity, fixed pre-planned seats, an
// exam overrunning (long/EXaHM Nachlauf) keeping its rooms in later slots, and — in
// summer — an own room never used in two directly consecutive slots (heat cooldown).
// Soft objective (weighted): place every seat (dominant), a free-seat buffer per exam,
// keep an exam together (few rooms), few distinct rooms overall (compaction), and — in
// summer — the later a slot, the lower the floor (Hitzeschutz).
package roomplan

import (
	"sort"
	"time"
)

// SeatKind distinguishes an ordinary student-seat from an NTA that needs a room alone.
// NTA students who sit with the cohort are modelled as ordinary Normal seats (their
// extended writing time is folded into the exam's occupancy window at build time).
type SeatKind int

const (
	Normal   SeatKind = iota // ordinary student (incl. NTA sitting in a normal room)
	NTAAlone                 // NTA that must occupy a room alone
)

// Seat is one student-seat to place. Mtknr identifies the student for the output (empty
// for an additionalSeats dummy). A Fixed seat is pre-planned onto FixedRoom and never
// moved. NTAAlone seats carry the NTA's Mtknr for the PlannedRoom.NtaMtknr field.
type Seat struct {
	Exam      int // index into Problem.Exams
	Mtknr     string
	Kind      SeatKind
	Fixed     bool
	FixedRoom int // room index when Fixed
}

// Room is a concrete room reduced to what the solver needs. Feature/availability/
// handicap matching is pre-computed into each exam's AllowedNormal/AllowedAlone lists,
// so the solver itself only needs the seat count plus the summer-heat attributes and
// the descriptive feature flags (kept for the reporting Check functions).
type Room struct {
	Name    string
	Seats   int
	OwnRoom bool // RequestWith == NONE: subject to the summer heat constraints
	// HeatLevel is the summer heat score (higher = hotter = worse late in the day):
	// the R-building floor from the name, or an explicit Hitzewert override; 0 for
	// booked/requested rooms (OwnRoom == false) and ground floor. Only used in summer.
	HeatLevel int

	// descriptive feature flags (for the reporting Check functions; enforcement is via
	// the pre-computed AllowedNormal/AllowedAlone lists)
	Exahm, Seb, Lab, Handicap, PlacesWithSocket bool
}

// Exam is one exam at a fixed slot. AllowedNormal/AllowedAlone are the room indices a
// Normal / NTAAlone seat of this exam may use, already filtered by feature, availability
// in the exam's slot and (for AllowedNormal) the handicap-rooms-only-for-NTA rule.
type Exam struct {
	Ancode   int
	Slot     int // index into Problem.Slots
	Duration int // minutes (for the PlannedRoom output and the room-turnaround check)
	// PreExtra / PostExtra are the setup (Vorlauf) / teardown (Nachlauf) minutes BEYOND the
	// ordinary room turnaround (0 for an exam on the default buffer; e.g. 15 for an EXaHM exam
	// with a 30-min buffer). They widen the required gap to a neighbouring use of the same room
	// (see State.turnaroundConflict).
	PreExtra, PostExtra int
	Exahm, Seb          bool
	NormalCount         int   // number of Normal seats (for the free-seat buffer target)
	AllowedNormal       []int // room indices allowed for Normal seats
	AllowedAlone        []int // room indices allowed for NTAAlone seats

	allowedNormalSet map[int]bool
	allowedAloneSet  map[int]bool
}

// Weights scale the soft objective. Placeholder values — to be calibrated against real
// data (Test26SS). Unplaced dominates so every seat is placed first.
type Weights struct {
	Unplaced   float64 // per unplaced seat (dominant)
	Buffer     float64 // per seat the free-seat buffer of an exam falls short
	Split      float64 // per extra room an exam is split across (keep it together)
	Compaction float64 // per distinct room used overall (request/open fewer rooms)
	HeatFloor  float64 // summer: per (heat level × lateness × seat) in an own room
	Churn      float64 // per seat whose room differs from the previous plan (warm start)

	// SebAvoidExahm penalizes, per seat, a SEB (non-EXaHM) exam placed in an EXaHM-capable
	// room, so SEB exams prefer plain SEB rooms (R-building) and leave the scarce booked
	// T-building EXaHM rooms for the exams that actually require EXaHM.
	SebAvoidExahm float64
	// OwnExahmFallback penalizes, per seat, an EXaHM exam placed in an OWN (R-building) EXaHM
	// room instead of a booked one — so an own EXaHM room (e.g. the 1-seat NTA room R1.011)
	// is used only as a fallback when no booked T-building EXaHM room is available.
	OwnExahmFallback float64

	// HeatBaselineHour is the clock hour up to which a slot start is "cool" (lateness 0);
	// later starts get lateness = start hour − baseline (hours). Only used in summer.
	HeatBaselineHour float64
}

// DefaultWeights returns placeholder weights (to be calibrated). Unplaced dominates;
// buffer/split/compaction are gentle shapers; the summer heat term is a mild nudge.
func DefaultWeights() Weights {
	return Weights{
		Unplaced:         1_000_000,
		Buffer:           50,
		Split:            30,
		Compaction:       20,
		HeatFloor:        5,
		Churn:            0,  // off by default; warm-start construction already limits churn
		SebAvoidExahm:    40, // keep booked EXaHM rooms free for EXaHM: push SEB to R-building
		OwnExahmFallback: 40, // prefer a booked EXaHM room; own R-building EXaHM room only as fallback
		HeatBaselineHour: 10,
	}
}

// Problem is the immutable input to the solver.
type Problem struct {
	Slots []Slot
	Rooms []Room
	Exams []Exam
	Seats []Seat
	W     Weights

	// Summer toggles the two heat constraints (heat-floor soft, summer-cooldown hard).
	// Set by the caller from the semester / room-heat mode; false leaves them inert.
	Summer bool

	// TimelagMin is the ordinary room turnaround (minutes) between two uses of the same room
	// (the exam's end to the next exam's start); widened per exam by Pre/PostExtra. 0 → 15.
	TimelagMin int

	// PrevRoom optionally holds, per seat index, the room it had in the saved plan
	// (-1 = none), for the churn soft term and the warm start. nil disables churn.
	PrevRoom []int

	// derived
	movable      []int     // seat indices that may move (not Fixed)
	seatsOfExam  [][]int   // exam -> its seat indices
	examsInSlot  [][]int   // slot -> exam indices running in it (for shared-cell buffer updates)
	sameDaySlots [][]int   // slot -> other slot indices on the same day (for the turnaround check)
	slotLateness []float64 // per slot: hours the start lies after HeatBaselineHour (0 = cool)
	dayOfSlot    []int     // per slot: calendar-day group index
	posInDay     []int     // per slot: chronological position within its day
	nextInDay    []int     // per slot: the next slot on the same day (-1 = none)
	prevInDay    []int     // per slot: the previous slot on the same day (-1 = none)
}

// timelag returns the ordinary room turnaround in minutes (default 15).
func (p *Problem) timelag() int {
	if p.TimelagMin > 0 {
		return p.TimelagMin
	}
	return 15
}

// Slot is a candidate placement time (an exam's fixed slot).
type Slot struct {
	Start time.Time
}

// NewProblem validates the input and precomputes the derived structures.
func NewProblem(slots []Slot, rooms []Room, exams []Exam, seats []Seat, w Weights) *Problem {
	p := &Problem{Slots: slots, Rooms: rooms, Exams: exams, Seats: seats, W: w}

	for i := range p.Exams {
		e := &p.Exams[i]
		e.allowedNormalSet = make(map[int]bool, len(e.AllowedNormal))
		for _, r := range e.AllowedNormal {
			e.allowedNormalSet[r] = true
		}
		e.allowedAloneSet = make(map[int]bool, len(e.AllowedAlone))
		for _, r := range e.AllowedAlone {
			e.allowedAloneSet[r] = true
		}
	}

	p.seatsOfExam = make([][]int, len(p.Exams))
	for i := range p.Seats {
		s := &p.Seats[i]
		p.seatsOfExam[s.Exam] = append(p.seatsOfExam[s.Exam], i)
		if !s.Fixed {
			p.movable = append(p.movable, i)
		}
	}

	p.examsInSlot = make([][]int, len(p.Slots))
	for e := range p.Exams {
		if s := p.Exams[e].Slot; s >= 0 && s < len(p.Slots) {
			p.examsInSlot[s] = append(p.examsInSlot[s], e)
		}
	}

	p.computeDayGrid()
	p.computeLateness()

	// same-day slot lists (excluding self), for the room-turnaround feasibility check: two
	// uses of one room must leave enough time between them, only ever relevant within a day.
	p.sameDaySlots = make([][]int, len(p.Slots))
	for s := range p.Slots {
		for t := range p.Slots {
			if t != s && p.dayOfSlot[t] == p.dayOfSlot[s] {
				p.sameDaySlots[s] = append(p.sameDaySlots[s], t)
			}
		}
	}
	return p
}

// computeDayGrid groups slots into calendar days (from their absolute Start) and records
// each slot's within-day chronological position plus its same-day neighbours — the basis
// for the summer cooldown ("directly consecutive" = adjacent within a day).
func (p *Problem) computeDayGrid() {
	byDay := make(map[int][]int)
	var dayKeys []int
	for i := range p.Slots {
		d := dayKey(p.Slots[i].Start)
		if _, ok := byDay[d]; !ok {
			dayKeys = append(dayKeys, d)
		}
		byDay[d] = append(byDay[d], i)
	}
	sort.Ints(dayKeys)
	p.dayOfSlot = make([]int, len(p.Slots))
	p.posInDay = make([]int, len(p.Slots))
	p.nextInDay = make([]int, len(p.Slots))
	p.prevInDay = make([]int, len(p.Slots))
	for i := range p.nextInDay {
		p.nextInDay[i] = -1
		p.prevInDay[i] = -1
	}
	for di, d := range dayKeys {
		day := byDay[d]
		sort.Slice(day, func(a, b int) bool { return p.Slots[day[a]].Start.Before(p.Slots[day[b]].Start) })
		for pos, s := range day {
			p.dayOfSlot[s] = di
			p.posInDay[s] = pos
			if pos > 0 {
				p.prevInDay[s] = day[pos-1]
			}
			if pos < len(day)-1 {
				p.nextInDay[s] = day[pos+1]
			}
		}
	}
}

// computeLateness sets slotLateness[s] = max(0, startHour − HeatBaselineHour) in hours,
// the monotone "how late in the day" factor the summer heat term multiplies the room's
// floor by ("je später, desto weiter unten").
func (p *Problem) computeLateness() {
	p.slotLateness = make([]float64, len(p.Slots))
	for i := range p.Slots {
		h := float64(p.Slots[i].Start.Hour()) + float64(p.Slots[i].Start.Minute())/60
		if late := h - p.W.HeatBaselineHour; late > 0 {
			p.slotLateness[i] = late
		}
	}
}

// allowsNormal / allowsAlone report whether room r is in the exam's allowed set for the
// respective seat kind.
func (p *Problem) allowsNormal(exam, r int) bool { return p.Exams[exam].allowedNormalSet[r] }
func (p *Problem) allowsAlone(exam, r int) bool  { return p.Exams[exam].allowedAloneSet[r] }

// allows reports whether seat i may use room r (dispatches on the seat's kind).
func (p *Problem) allows(i, r int) bool {
	s := &p.Seats[i]
	if s.Kind == NTAAlone {
		return p.allowsAlone(s.Exam, r)
	}
	return p.allowsNormal(s.Exam, r)
}

// heatCostOf is the summer heat penalty of placing a Normal seat of the given exam into
// room r: HeatFloor × room heat level × slot lateness. 0 outside summer, for booked
// rooms, NTA-alone seats or an unplaced seat.
func (p *Problem) heatCostOf(i, r int) float64 {
	if !p.Summer || r < 0 || p.W.HeatFloor == 0 {
		return 0
	}
	room := &p.Rooms[r]
	if !room.OwnRoom || room.HeatLevel == 0 || p.Seats[i].Kind == NTAAlone {
		return 0
	}
	return p.W.HeatFloor * float64(room.HeatLevel) * p.slotLateness[p.Exams[p.Seats[i].Exam].Slot]
}

// sebAvoidCostOf is the SEB-in-EXaHM-room penalty for placing seat i into room r: W.SebAvoidExahm
// when the seat's exam is SEB (and not EXaHM) and r is an EXaHM-capable room. Keeps the booked
// T-building EXaHM rooms free for EXaHM exams (SEB can also run in plain R-building SEB rooms).
func (p *Problem) sebAvoidCostOf(i, r int) float64 {
	if r < 0 || p.W.SebAvoidExahm == 0 {
		return 0
	}
	e := p.Seats[i].Exam
	if p.Exams[e].Seb && !p.Exams[e].Exahm && p.Rooms[r].Exahm {
		return p.W.SebAvoidExahm
	}
	return 0
}

// ownExahmCostOf is the own-EXaHM-room fallback penalty for placing seat i into room r:
// W.OwnExahmFallback when the seat's exam requires EXaHM and r is an OWN (R-building) EXaHM room
// (e.g. the 1-seat NTA room R1.011). Prefers the booked T-building EXaHM rooms; the own room is
// used only when no booked one is available.
func (p *Problem) ownExahmCostOf(i, r int) float64 {
	if r < 0 || p.W.OwnExahmFallback == 0 {
		return 0
	}
	e := p.Seats[i].Exam
	if p.Exams[e].Exahm && p.Rooms[r].Exahm && p.Rooms[r].OwnRoom {
		return p.W.OwnExahmFallback
	}
	return 0
}

// FloorFromName extracts the R-building floor from a room name of the form "Rx.abc"
// (x = floor digit), e.g. "R2.007" → 2, "R0.011" → 0. Names not matching that pattern
// (other buildings, online rooms) return 0. It is the default heat level for an own room
// when no explicit Hitzewert override is set; kept here so the build step and the tests
// share one definition.
func FloorFromName(name string) int {
	if len(name) < 3 || (name[0] != 'R' && name[0] != 'r') || name[2] != '.' {
		return 0
	}
	c := name[1]
	if c < '0' || c > '9' {
		return 0
	}
	return int(c - '0')
}

func dayKey(t time.Time) int {
	y, m, d := t.Date()
	return y*10000 + int(m)*100 + d
}
