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
	"sort"
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
	ExahmSeats int // booked (T-building) EXaHM seats this slot
	SebSeats   int // booked (T-building) SEB seats this slot
}

// Unit is one schedulable item. A "same slot" group is pre-merged into a single Unit
// (its Ancodes then holds all members). Fixed units (locked / external /
// notPlannedByMe) keep FixedSlot and are never moved.
type Unit struct {
	ID      int   // representative ancode
	Ancodes []int // all ancodes in this (same-slot) unit
	Seats   int   // students to seat this slot (sum over members)
	Exahm   bool
	Seb     bool
	Examer  int    // main examer id, for same-examer clustering (0 = unknown)
	Module  string // for section clustering
	Program string

	Allowed   []int // allowed slot indices; empty = all slots allowed
	Fixed     bool
	FixedSlot int
	// StartSlot is the warm-start slot (this exam's current plan entry), -1 if none.
	// Used only when solving with warmStart to begin from the existing plan.
	StartSlot int
	// Location is the exam's campus (empty = default). Exams at different campuses need
	// travel time, so a same-day pair across campuses is penalized extra.
	Location string
	// Foreign: planned by another faculty (external / notPlannedByMe). A conflict
	// between two Foreign exams is neither optimizable nor our problem, so it is left
	// out of the objective and the diagnostics; our own fixed (pre-planned) exams are
	// NOT Foreign.
	Foreign bool

	allowedSet map[int]bool
}

// Pair is a conflict between two of a student's units (A<B), already weighted by the
// shared-student contribution and any per-student repeat discount. Pairs a student
// may legitimately never sit together (canShareSlot) are omitted at build time.
type Pair struct {
	A, B   int
	Weight float64
	// CrossLoc: the two exams are at different campuses (extra travel-gap penalty when
	// they end up on the same day).
	CrossLoc bool
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
	CrossCampus   float64 // extra penalty for a same-day student pair across campuses (travel gap)
	TbauFill      float64 // per unused booked T-building seat (EXaHM/SEB phase A: fill the rooms)
	Hole          float64 // per empty slot that lies between two occupied slots on the same day
	// TimeOfDay scales the start-time avoidance penalty: per seat, per hour a slot's start
	// time lies outside the wanted window (see Problem.TimeSeverity). 0 = off. The window
	// is semester-dependent (winter: avoid early starts; summer: avoid late starts) and is
	// baked into TimeSeverity by the caller, so the solver only needs the scalar weight.
	TimeOfDay float64
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
		SlotLoad:      2, // even distribution over slots (deviation from ideal load)²
		LoadThreshold: 200,
		Unplaced:      1_000_000,
		CrossCampus:   3000,
		TbauFill:      0,    // off by default (Phase B); the EXaHM/SEB phase A sets it high
		Hole:          1500, // an empty slot mid-day is bad for invigilation planning; drive it to the day edge (or fill it). Below Adjacent (2500) so it never creates a directly-consecutive pair just to close a hole; above SameDay (900) so it may accept a mild same-day proximity.
		TimeOfDay:     0,    // off by default; the caller sets it (and TimeSeverity) per semester
	}
}

// tbauPenalty is the T-building fill penalty for a slot given the EXaHM/SEB seats
// currently placed there: unused booked seats (EXaHM + SEB) times TbauFill. Minimizing
// it fills the booked rooms; SEB overflow (used > booked) is allowed (goes to R-rooms).
func (p *Problem) tbauPenalty(slotIdx, exahmUsed, sebUsed int) float64 {
	if p.W.TbauFill == 0 {
		return 0
	}
	unused := 0
	if d := p.Slots[slotIdx].ExahmSeats - exahmUsed; d > 0 {
		unused += d
	}
	if d := p.Slots[slotIdx].SebSeats - sebUsed; d > 0 {
		unused += d
	}
	return p.W.TbauFill * float64(unused)
}

// SetTimeSeverity installs the per-slot start-time avoidance severity (see
// Problem.TimeSeverity). len(sev) must equal len(Slots); a shorter/nil slice or a zero
// W.TimeOfDay simply disables the penalty. Call before Solve.
func (p *Problem) SetTimeSeverity(sev []float64) {
	p.TimeSeverity = sev
}

// timePenalty is the start-time avoidance penalty for placing `seats` students into slot
// slotIdx: W.TimeOfDay * severity(slot) * seats. slotIdx < 0 (unplaced) costs nothing.
func (p *Problem) timePenalty(seats, slotIdx int) float64 {
	if p.W.TimeOfDay == 0 || slotIdx < 0 || slotIdx >= len(p.TimeSeverity) {
		return 0
	}
	return p.W.TimeOfDay * p.TimeSeverity[slotIdx] * float64(seats)
}

// Problem is the immutable input to the solver.
type Problem struct {
	Slots    []Slot
	Units    []Unit
	Students []Student
	Attract  []AttractPair
	W        Weights

	// TimeSeverity is the per-slot start-time avoidance severity (index-aligned with
	// Slots): how far, in hours, the slot's start time lies outside the wanted window
	// (0 = inside / no penalty). Semester-dependent and set by the caller via
	// SetTimeSeverity; combined with W.TimeOfDay and a unit's seats into timePenalty.
	// nil (or W.TimeOfDay == 0) disables the term.
	TimeSeverity []float64

	// derived
	movable        []int
	hardConf       []map[int]bool // unit -> units that must not overlap it in time
	hardConfSorted [][]int        // deterministic (sorted) view of hardConf for float sums
	// hardSep[u][v] = minimum minutes from u's start until v may start, i.e. u's occupied
	// time (its duration, extended for the worst shared student's NTA) plus the travel/
	// break buffer. Two conflicting exams must not overlap: v is only allowed at or after
	// u.start+hardSep[u][v], or u at or after v.start+hardSep[v][u]. Missing entry ⇒ the
	// minimal separation (same start time forbidden, any different time allowed). Set via
	// SetHardSeparations; on the fixed grid this reproduces the old same-slot + NTA-overrun
	// behaviour, but it is correct at any start-time granularity.
	hardSep      []map[int]int
	unitStudents [][]int        // unit -> student indices that have a pair with it
	unitAttract  [][]attractRef // unit -> its attract partners
	targetLoad   float64        // ideal seats per slot = total seats / number of slots
	// day grouping for the interior-hole penalty: days[d] is the slot indices of one
	// exam day, sorted by slot number; dayOfSlot maps a slot index back to its day group.
	days      [][]int
	dayOfSlot []int
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

	if len(p.Slots) > 0 {
		totalSeats := 0
		for i := range p.Units {
			totalSeats += p.Units[i].Seats
		}
		p.targetLoad = float64(totalSeats) / float64(len(p.Slots))
	}

	// units that must not share a slot: every counted student pair (canShareSlot pairs
	// are already omitted from Student.Pairs at build time).
	p.hardConf = make([]map[int]bool, len(p.Units))
	p.unitStudents = make([][]int, len(p.Units))
	for si := range p.Students {
		units := make(map[int]bool)
		for _, pr := range p.Students[si].Pairs {
			// two fixed exams can never be separated, so they need no same-slot veto
			// (this also keeps a pre-planned same-slot clash from blocking the write);
			// they still count in the diagnostics.
			if !p.Units[pr.A].Fixed || !p.Units[pr.B].Fixed {
				addConf(p.hardConf, pr.A, pr.B)
				addConf(p.hardConf, pr.B, pr.A)
			}
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

	// day grouping (for the interior-hole penalty): collect slot indices per exam day,
	// each sorted by slot number, days themselves in ascending day order.
	byDay := make(map[int][]int)
	dayNums := make([]int, 0)
	for i := range p.Slots {
		d := p.Slots[i].Day
		if _, ok := byDay[d]; !ok {
			dayNums = append(dayNums, d)
		}
		byDay[d] = append(byDay[d], i)
	}
	sort.Ints(dayNums)
	p.dayOfSlot = make([]int, len(p.Slots))
	p.days = make([][]int, 0, len(dayNums))
	for di, d := range dayNums {
		slots := byDay[d]
		sort.Slice(slots, func(a, b int) bool { return p.Slots[slots[a]].Slot < p.Slots[slots[b]].Slot })
		for _, s := range slots {
			p.dayOfSlot[s] = di
		}
		p.days = append(p.days, slots)
	}

	// deterministic (sorted) view of hardConf: iterating a Go map has random order, so
	// summing floats over it (constructive addedCost) would make runs non-reproducible.
	p.hardConfSorted = make([][]int, len(p.Units))
	for u := range p.hardConf {
		if len(p.hardConf[u]) == 0 {
			continue
		}
		lst := make([]int, 0, len(p.hardConf[u]))
		for v := range p.hardConf[u] {
			lst = append(lst, v)
		}
		sort.Ints(lst)
		p.hardConfSorted[u] = lst
	}
	return p
}

// SetHardSeparations installs the per-pair minimum time separations used by the
// time-overlap hard constraint. sep[[2]int{u,v}] is the number of minutes from u's start
// until v may start (u's occupied time — its duration, extended for the worst shared
// student's NTA — plus the travel/break buffer). Only entries whose units are conflict
// partners (present in hardConf) take effect; a both-fixed pair is ignored (neither can
// move to satisfy it). Idempotent; replaces any previously installed separations.
func (p *Problem) SetHardSeparations(sep map[[2]int]int) {
	p.hardSep = make([]map[int]int, len(p.Units))
	for key, minutes := range sep {
		u, v := key[0], key[1]
		if u < 0 || v < 0 || u >= len(p.Units) || v >= len(p.Units) || u == v {
			continue
		}
		if p.Units[u].Fixed && p.Units[v].Fixed {
			continue
		}
		if p.hardSep[u] == nil {
			p.hardSep[u] = make(map[int]int)
		}
		p.hardSep[u][v] = minutes
	}
}

// sepMinutes returns the required minutes from u's start until v may start: the installed
// separation, or 1 (only the same start time is forbidden) when none is set.
func (p *Problem) sepMinutes(u, v int) int {
	if p.hardSep != nil && p.hardSep[u] != nil {
		if m, ok := p.hardSep[u][v]; ok {
			return m
		}
	}
	return 1
}

// NumHardSeparations returns the number of installed pair separations (both directions).
func (p *Problem) NumHardSeparations() int {
	n := 0
	for _, m := range p.hardSep {
		n += len(m)
	}
	return n
}

// overlaps reports whether units u and v, placed in slots su and sv, would overlap in
// time — i.e. leave less than the required separation between them (same start time, or
// the earlier exam's occupied time + buffer reaching into the later one). Symmetric.
func (p *Problem) overlaps(u, su, v, sv int) bool {
	d := int(p.Slots[sv].Start.Sub(p.Slots[su].Start).Minutes()) // minutes v starts after u
	if d >= 0 {
		return d < p.sepMinutes(u, v)
	}
	return -d < p.sepMinutes(v, u)
}

// loadPenalty is the even-distribution penalty for a slot holding `seats` seats: the
// squared deviation from the ideal load (total seats / number of slots), so both empty
// and very full slots are penalized and the solver spreads exams evenly over the slots.
func (p *Problem) loadPenalty(seats int) float64 {
	d := float64(seats) - p.targetLoad
	return p.W.SlotLoad * d * d
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
// placed, a != b for a counted pair). Decreasing with temporal distance. Same-day pairs
// use the fixed Adjacent/SameDay tiers; across days it falls off with the REAL time gap
// (in hours), so an exam at 16:30 followed by one the next morning at 08:30 (16 h) costs
// more than 08:30 → next 16:30 (32 h), and a weekend gap (many hours) is naturally cheap.
func (p *Problem) closeness(a, b int) float64 {
	sa, sb := p.Slots[a].SlotRef, p.Slots[b].SlotRef
	if sa.Day == sb.Day {
		if abs(sa.Slot-sb.Slot) == 1 {
			return p.W.Adjacent
		}
		return p.W.SameDay
	}
	hours := math.Abs(sa.Start.Sub(sb.Start).Hours())
	if hours < 1 {
		hours = 1
	}
	return p.W.DayFactor * 24.0 / hours
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
