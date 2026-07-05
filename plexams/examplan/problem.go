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

// Problem is the immutable input to the solver.
type Problem struct {
	Slots    []Slot
	Units    []Unit
	Students []Student
	Attract  []AttractPair
	W        Weights

	// derived
	movable        []int
	hardConf       []map[int]bool // unit -> units that must not share its slot
	hardConfSorted [][]int        // deterministic (sorted) view of hardConf for float sums
	unitStudents   [][]int        // unit -> student indices that have a pair with it
	unitAttract    [][]attractRef // unit -> its attract partners
	targetLoad     float64        // ideal seats per slot = total seats / number of slots
	// NTA time-overrun (hard): nextSlot/prevSlot map a slot to its same-day neighbour
	// (-1 if none); overrunNext[a] = units that must not sit in the slot right after a,
	// overrunPrev[b] = units a such that b must not sit right after a. Always allocated
	// (len == #units); empty unless SetNTAOverruns installed pairs.
	nextSlot    []int
	prevSlot    []int
	overrunNext [][]int
	overrunPrev [][]int
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

	// same-day slot neighbours (for the NTA time-overrun constraint)
	byDaySlot := make(map[[2]int]int, len(p.Slots))
	for i := range p.Slots {
		byDaySlot[[2]int{p.Slots[i].Day, p.Slots[i].Slot}] = i
	}
	p.nextSlot = make([]int, len(p.Slots))
	p.prevSlot = make([]int, len(p.Slots))
	for i := range p.Slots {
		d, s := p.Slots[i].Day, p.Slots[i].Slot
		p.nextSlot[i], p.prevSlot[i] = -1, -1
		if j, ok := byDaySlot[[2]int{d, s + 1}]; ok {
			p.nextSlot[i] = j
		}
		if j, ok := byDaySlot[[2]int{d, s - 1}]; ok {
			p.prevSlot[i] = j
		}
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

	// allocated but empty until SetNTAOverruns is called (feasible() ranges over them)
	p.overrunNext = make([][]int, len(p.Units))
	p.overrunPrev = make([][]int, len(p.Units))

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

// SetNTAOverruns installs the consecutive-exam-gap constraints: for each ordered pair
// (a, b), unit b must not be placed in the slot immediately following a on the same day,
// because a student sharing both is still occupied with a (its duration — extended for
// that student's NTA — plus the travel buffer) when that next slot starts. Both-fixed
// pairs are ignored (neither can be moved to satisfy it). Idempotent; replaces any
// previously installed pairs.
func (p *Problem) SetNTAOverruns(pairs [][2]int) {
	nextSet := make([]map[int]bool, len(p.Units))
	prevSet := make([]map[int]bool, len(p.Units))
	for _, pr := range pairs {
		a, b := pr[0], pr[1]
		if a < 0 || b < 0 || a >= len(p.Units) || b >= len(p.Units) || a == b {
			continue
		}
		if p.Units[a].Fixed && p.Units[b].Fixed {
			continue
		}
		if nextSet[a] == nil {
			nextSet[a] = make(map[int]bool)
		}
		nextSet[a][b] = true
		if prevSet[b] == nil {
			prevSet[b] = make(map[int]bool)
		}
		prevSet[b][a] = true
	}
	p.overrunNext = sortedSets(nextSet)
	p.overrunPrev = sortedSets(prevSet)
}

// NumNTAOverruns returns the number of installed NTA time-overrun adjacency pairs.
func (p *Problem) NumNTAOverruns() int {
	n := 0
	for _, l := range p.overrunNext {
		n += len(l)
	}
	return n
}

// sortedSets converts a per-unit set of unit indices into a deterministic sorted slice.
func sortedSets(sets []map[int]bool) [][]int {
	out := make([][]int, len(sets))
	for i, m := range sets {
		if len(m) == 0 {
			continue
		}
		lst := make([]int, 0, len(m))
		for v := range m {
			lst = append(lst, v)
		}
		sort.Ints(lst)
		out[i] = lst
	}
	return out
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
