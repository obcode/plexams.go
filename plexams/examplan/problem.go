// Package examplan generates the exam schedule (Terminplan): it assigns every exam
// (unit) to a time slot on top of the generic simulated-annealing core in
// plexams/optimize. It is deliberately DB-agnostic — the caller builds a *Problem
// from the domain data and maps the resulting slot assignment back to plan entries.
//
// The primary objective is to spread each student's exams as far apart as possible.
// Hard constraints: fixed placements stay put, a student never has two exams in the
// same slot (unless the pair is declared shareable), per-slot EXaHM and total seat
// capacities. Soft objective (weighted): the spread (sum over students + a convex
// worst-case term so no single student gets a bad schedule), "attract" pairs that
// should sit together (parallel sections / small exams of the same examer), and a
// slot-load term keeping the seats/rooms per slot moderate.
package examplan

import (
	"math"
	"time"
)

// SlotRef identifies a slot in the exam period; Start is its absolute start time,
// used so temporal distance naturally includes weekends (Fri->Mon is farther than
// Tue->Wed even though the day numbers are consecutive).
type SlotRef struct {
	Day   int // 1-based exam day number
	Slot  int // 1-based slot number within the day
	Start time.Time
}

// Slot is a candidate slot with its capacities. Seats/ExahmSeats of 0 mean "no
// limit known" and are not enforced.
type Slot struct {
	SlotRef
	Seats      int // total room seats available this slot (global capacity)
	ExahmSeats int // booked EXaHM seats this slot
}

// Unit is one schedulable item. A "same slot" group is pre-merged into a single Unit
// (its Ancodes then holds all members). Fixed units (locked / external /
// notPlannedByMe) keep FixedSlot and are never moved.
type Unit struct {
	ID      int   // representative ancode
	Ancodes []int // all ancodes in this (same-slot) unit
	Seats   int   // students to seat this slot (sum over members)
	Exahm   bool
	Examer  int    // main examer id, for same-examer clustering (0 = unknown)
	Module  string // for section clustering
	Program string

	Allowed   []int // allowed slot indices; empty = all slots allowed
	Fixed     bool
	FixedSlot int

	allowedSet map[int]bool
}

// Pair is a conflict between two of a student's units (A<B), already weighted by the
// shared-student contribution and any per-student repeat discount. Pairs a student
// may legitimately never sit together (canShareSlot) are omitted at build time.
type Pair struct {
	A, B   int
	Weight float64
}

// Student is one student's conflicting unit pairs, used for the spread objective.
type Student struct {
	ID    string
	Pairs []Pair
}

// AttractPair are two units that should sit close (ideally same slot): parallel
// sections of the same module, or small exams of the same examer.
type AttractPair struct {
	A, B   int
	Weight float64
}

// Weights scale the soft objective. Placeholder values — to be calibrated against
// real data (Test26SS).
type Weights struct {
	Adjacent      float64 // same day, adjacent slot (very bad)
	SameDay       float64 // same day, non-adjacent
	DayFactor     float64 // different days: DayFactor / calendarDaysBetween
	WorstCase     float64 // convex per-student term: + WorstCase * P_s^2
	RepeatFactor  float64 // multiplier applied (at build time) to repeat-tolerable pairs
	Attract       float64 // per distance rank for attract pairs (want them close)
	SlotLoad      float64 // convex penalty per seat over LoadThreshold in a slot
	LoadThreshold int     // soft seat threshold per slot
	Unplaced      float64 // penalty per unplaced unit (dominant)
}

// DefaultWeights returns the calibrated weights (tuned against real data, Test26SS,
// 2026-07-02: Adjacent/SameDay high enough to push directly-consecutive to 0 and
// same-day exams down markedly, with a mild worst-case term protecting the least
// well-spread students). Ordered so the spread dominates and clustering / slot-load
// are tie-breakers; Unplaced dominates everything so the solver places all exams first.
func DefaultWeights() Weights {
	return Weights{
		Adjacent:      2500,
		SameDay:       900,
		DayFactor:     200,
		WorstCase:     0.05,
		RepeatFactor:  0.3,
		Attract:       50,
		SlotLoad:      0.5,
		LoadThreshold: 200,
		Unplaced:      1_000_000,
	}
}

// Problem is the immutable input to the solver.
type Problem struct {
	Slots    []Slot
	Units    []Unit
	Students []Student
	Attract  []AttractPair
	W        Weights

	// derived
	movable      []int
	hardConf     []map[int]bool // unit -> units that must not share its slot
	unitStudents [][]int        // unit -> student indices that have a pair with it
	unitAttract  [][]attractRef // unit -> its attract partners

	sep       [][2]int // unit pairs that must be on different days ("unzulässig")
	sepByUnit [][]int  // unit -> units it must be on a different day from
}

// SetSeparated declares unit pairs that must be scheduled on different days (the hard
// effect of an "unzulässig" conflict rating). Call after NewProblem.
func (p *Problem) SetSeparated(pairs [][2]int) {
	p.sep = pairs
	p.sepByUnit = make([][]int, len(p.Units))
	for _, pr := range pairs {
		a, b := pr[0], pr[1]
		if a < 0 || b < 0 || a >= len(p.Units) || b >= len(p.Units) {
			continue
		}
		p.sepByUnit[a] = append(p.sepByUnit[a], b)
		p.sepByUnit[b] = append(p.sepByUnit[b], a)
	}
}

type attractRef struct {
	other  int
	weight float64
}

// NewProblem validates the input and precomputes derived structures.
func NewProblem(slots []Slot, units []Unit, students []Student, attract []AttractPair, w Weights) *Problem {
	p := &Problem{Slots: slots, Units: units, Students: students, Attract: attract, W: w}

	for i := range p.Units {
		u := &p.Units[i]
		if len(u.Allowed) > 0 {
			u.allowedSet = make(map[int]bool, len(u.Allowed))
			for _, s := range u.Allowed {
				u.allowedSet[s] = true
			}
		}
		if !u.Fixed {
			p.movable = append(p.movable, i)
		}
	}

	// units that must not share a slot: every counted student pair (canShareSlot pairs
	// are already omitted from Student.Pairs at build time).
	p.hardConf = make([]map[int]bool, len(p.Units))
	p.unitStudents = make([][]int, len(p.Units))
	for si := range p.Students {
		units := make(map[int]bool)
		for _, pr := range p.Students[si].Pairs {
			addConf(p.hardConf, pr.A, pr.B)
			addConf(p.hardConf, pr.B, pr.A)
			units[pr.A] = true
			units[pr.B] = true
		}
		// each unit appears once in this student's set, so no duplicate student ids.
		for u := range units {
			p.unitStudents[u] = append(p.unitStudents[u], si)
		}
	}

	p.unitAttract = make([][]attractRef, len(p.Units))
	for _, ap := range p.Attract {
		p.unitAttract[ap.A] = append(p.unitAttract[ap.A], attractRef{ap.B, ap.Weight})
		p.unitAttract[ap.B] = append(p.unitAttract[ap.B], attractRef{ap.A, ap.Weight})
	}
	return p
}

// loadPenalty is the convex slot-load penalty for a slot holding `seats` seats.
func (p *Problem) loadPenalty(seats int) float64 {
	over := seats - p.W.LoadThreshold
	if over <= 0 {
		return 0
	}
	return p.W.SlotLoad * float64(over) * float64(over)
}

func addConf(m []map[int]bool, a, b int) {
	if m[a] == nil {
		m[a] = make(map[int]bool)
	}
	m[a][b] = true
}

// allows reports whether slot index s is in unit u's domain.
func (p *Problem) allows(u, s int) bool {
	set := p.Units[u].allowedSet
	return set == nil || set[s]
}

// closeness is the spread penalty for a counted pair placed in slots a and b (both
// placed, a != b for a counted pair). Decreasing with temporal distance.
func (p *Problem) closeness(a, b int) float64 {
	sa, sb := p.Slots[a].SlotRef, p.Slots[b].SlotRef
	if sa.Day == sb.Day {
		if abs(sa.Slot-sb.Slot) == 1 {
			return p.W.Adjacent
		}
		return p.W.SameDay
	}
	return p.W.DayFactor / float64(calDays(sa.Start, sb.Start))
}

// farness is the attract penalty (we want the pair close): 0 in the same slot, then
// growing with distance so the solver pulls them together.
func (p *Problem) farness(a, b int) float64 {
	if a == b {
		return 0
	}
	sa, sb := p.Slots[a].SlotRef, p.Slots[b].SlotRef
	if sa.Day == sb.Day {
		return p.W.Attract * float64(abs(sa.Slot-sb.Slot))
	}
	return p.W.Attract * float64(calDays(sa.Start, sb.Start)+5) // different day: at least beyond any intra-day distance
}

func calDays(a, b time.Time) int {
	d := a.Sub(b)
	if d < 0 {
		d = -d
	}
	days := int(math.Round(d.Hours() / 24))
	if days < 1 {
		days = 1
	}
	return days
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
