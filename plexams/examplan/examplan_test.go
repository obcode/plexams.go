package examplan

import (
	"math/rand"
	"testing"
	"time"

	"github.com/obcode/plexams.go/plexams/optimize"
)

// oneDaySlots builds n consecutive same-day slots (2h apart) with unlimited seats.
func oneDaySlots(n int) []Slot {
	t0 := time.Date(2026, 7, 6, 8, 30, 0, 0, time.UTC)
	slots := make([]Slot, n)
	for i := 0; i < n; i++ {
		slots[i] = Slot{SlotRef: SlotRef{Day: 1, Slot: i + 1, Start: t0.Add(time.Duration(2*i) * time.Hour)}, Seats: 1000}
	}
	return slots
}

func testSlots() []Slot {
	t0 := time.Date(2026, 7, 6, 8, 30, 0, 0, time.UTC) // Mon
	mk := func(day, slot, addH int) Slot {
		return Slot{SlotRef: SlotRef{Day: day, Slot: slot, Start: t0.Add(time.Duration(addH) * time.Hour)}, Seats: 1000}
	}
	return []Slot{
		mk(1, 1, 0),  // idx0 Mon 08:30
		mk(1, 2, 3),  // idx1 Mon 11:30
		mk(2, 1, 24), // idx2 Tue 08:30
		mk(2, 2, 27), // idx3 Tue 11:30
	}
}

func fastOpts() optimize.Options {
	o := optimize.DefaultOptions()
	o.Iterations = 30_000
	o.StartTemp = 500
	o.EndTemp = 0.01
	o.StopWhenConverged = false
	return o
}

func TestSolveSpreadsConflictingExams(t *testing.T) {
	units := []Unit{
		{ID: 1, Ancodes: []int{1}, Seats: 10},
		{ID: 2, Ancodes: []int{2}, Seats: 10},
	}
	students := make([]Student, 20)
	for i := range students {
		students[i] = Student{ID: string(rune('a' + i)), Pairs: []Pair{{A: 0, B: 1, Weight: 1}}}
	}
	p := NewProblem(testSlots(), units, students, nil, DefaultWeights())

	st, _ := Solve(p, fastOpts(), false)

	if st.SlotOf[0] < 0 || st.SlotOf[1] < 0 {
		t.Fatalf("exams not placed: %v", st.SlotOf)
	}
	if st.SlotOf[0] == st.SlotOf[1] {
		t.Fatalf("conflicting exams in same slot (hard violation): %v", st.SlotOf)
	}
	// best spread puts them on different days
	if p.Slots[st.SlotOf[0]].Day == p.Slots[st.SlotOf[1]].Day {
		t.Errorf("conflicting exams not spread to different days: slots %v", st.SlotOf)
	}
	if vs := p.Registry().HardViolations(st); len(vs) != 0 {
		t.Errorf("unexpected hard violations: %+v", vs)
	}
}

func TestNTAOverrunBlocksAdjacentSlot(t *testing.T) {
	units := []Unit{{ID: 1, Ancodes: []int{1}, Seats: 1}, {ID: 2, Ancodes: []int{2}, Seats: 1}}
	p := NewProblem(testSlots(), units, nil, nil, DefaultWeights())
	p.SetNTAOverruns([][2]int{{0, 1}}) // unit 1 must not sit right after unit 0

	// A (unit 0) in Mon slot 1 (idx0): B (unit 1) may not go into Mon slot 2 (idx1),
	// but may go to the next day (idx2/idx3).
	st := newState(p)
	st.setPhysical(0, 0)
	if st.feasible(1, 1) {
		t.Errorf("B must not be feasible in the slot right after A")
	}
	if !st.feasible(1, 2) || !st.feasible(1, 3) {
		t.Errorf("B must be feasible in non-adjacent slots")
	}

	// reverse direction: A overruns, so A may not sit right before an already-placed B.
	st2 := newState(p)
	st2.setPhysical(1, 1) // B in Mon slot 2 (idx1)
	if st2.feasible(0, 0) {
		t.Errorf("A must not be feasible right before B (its overrun reaches B's slot)")
	}
}

func TestSolveRespectsNTAOverrun(t *testing.T) {
	t0 := time.Date(2026, 7, 6, 8, 30, 0, 0, time.UTC)
	mk := func(slot, addH int) Slot {
		return Slot{SlotRef: SlotRef{Day: 1, Slot: slot, Start: t0.Add(time.Duration(addH) * time.Hour)}, Seats: 1000}
	}
	slots := []Slot{mk(1, 0), mk(2, 2), mk(3, 4)} // three consecutive same-day slots
	units := []Unit{
		{ID: 1, Ancodes: []int{1}, Seats: 1, Allowed: []int{0}},    // A pinned to slot 1 by domain
		{ID: 2, Ancodes: []int{2}, Seats: 1, Allowed: []int{1, 2}}, // B may take slot 2 or 3
	}
	p := NewProblem(slots, units, nil, nil, DefaultWeights())
	p.SetNTAOverruns([][2]int{{0, 1}})

	st, _ := Solve(p, fastOpts(), false)
	if st.SlotOf[0] != 0 {
		t.Fatalf("A expected in slot idx0, got %d", st.SlotOf[0])
	}
	if st.SlotOf[1] != 2 {
		t.Errorf("B must skip the slot right after A (idx1) and land in idx2, got %d", st.SlotOf[1])
	}
	if vs := p.Registry().HardViolations(st); len(vs) != 0 {
		t.Errorf("unexpected hard violations: %+v", vs)
	}
}

func TestSolveRespectsExahmCapacity(t *testing.T) {
	slots := testSlots()
	slots[2].ExahmSeats = 10 // only idx2 is an EXaHM slot

	units := []Unit{
		{ID: 1, Ancodes: []int{1}, Seats: 10, Exahm: true},
		{ID: 2, Ancodes: []int{2}, Seats: 10}, // normal
	}
	p := NewProblem(slots, units, nil, nil, DefaultWeights())

	st, _ := Solve(p, fastOpts(), false)

	if st.SlotOf[0] != 2 {
		t.Errorf("EXaHM exam must land in the only EXaHM slot (idx2), got %d", st.SlotOf[0])
	}
	if vs := p.Registry().HardViolations(st); len(vs) != 0 {
		t.Errorf("unexpected hard violations: %+v", vs)
	}
}

func TestSolveKeepsFixedAndAttracts(t *testing.T) {
	// two parallel sections that should sit together; one is fixed to idx0.
	units := []Unit{
		{ID: 1, Ancodes: []int{1}, Seats: 10, Fixed: true, FixedSlot: 0},
		{ID: 2, Ancodes: []int{2}, Seats: 10},
	}
	attract := []AttractPair{{A: 0, B: 1, Weight: 1}}
	w := DefaultWeights()
	w.SlotLoad = 0 // isolate the attract term from the even-distribution term
	p := NewProblem(testSlots(), units, nil, attract, w)

	st, _ := Solve(p, fastOpts(), false)

	if st.SlotOf[0] != 0 {
		t.Errorf("fixed exam moved: %v", st.SlotOf)
	}
	if st.SlotOf[1] != 0 {
		t.Errorf("attract pair not pulled to the same slot as its fixed section: %v", st.SlotOf)
	}
}

func TestSolveDeterministic(t *testing.T) {
	build := func() *Problem {
		units := []Unit{
			{ID: 1, Ancodes: []int{1}, Seats: 10},
			{ID: 2, Ancodes: []int{2}, Seats: 10},
			{ID: 3, Ancodes: []int{3}, Seats: 10},
		}
		students := []Student{
			{ID: "a", Pairs: []Pair{{A: 0, B: 1, Weight: 1}, {A: 1, B: 2, Weight: 1}}},
		}
		return NewProblem(testSlots(), units, students, nil, DefaultWeights())
	}
	sa, _ := Solve(build(), fastOpts(), false)
	sb, _ := Solve(build(), fastOpts(), false)
	for i := range sa.SlotOf {
		if sa.SlotOf[i] != sb.SlotOf[i] {
			t.Fatalf("not deterministic at unit %d: %v vs %v", i, sa.SlotOf, sb.SlotOf)
		}
	}
}

func TestWarmStartUsesCurrentAssignment(t *testing.T) {
	// the warm-start construction begins from the exams' current slots (StartSlot),
	// rather than reconstructing greedily from scratch.
	units := []Unit{
		{ID: 1, Ancodes: []int{1}, Seats: 10, StartSlot: 2}, // Tue 08:30
		{ID: 2, Ancodes: []int{2}, Seats: 10, StartSlot: 0}, // Mon 08:30
	}
	students := []Student{{ID: "a", Pairs: []Pair{{A: 0, B: 1, Weight: 1}}}}
	p := NewProblem(testSlots(), units, students, nil, DefaultWeights())

	st := constructWarm(p)
	if st.SlotOf[0] != 2 || st.SlotOf[1] != 0 {
		t.Errorf("warm start did not begin from the current assignment: got %v, want [2 0]", st.SlotOf)
	}
	if vs := p.Registry().HardViolations(st); len(vs) != 0 {
		t.Errorf("unexpected hard violations in warm start: %+v", vs)
	}
}

func TestWarmStartRepairsInfeasibleStart(t *testing.T) {
	// the current plan puts two conflicting exams in the SAME slot (a hard violation,
	// e.g. after a data change): warm start keeps one, moves the other to a feasible slot.
	units := []Unit{
		{ID: 1, Ancodes: []int{1}, Seats: 10, StartSlot: 0},
		{ID: 2, Ancodes: []int{2}, Seats: 10, StartSlot: 0}, // same slot as unit 0
	}
	students := []Student{{ID: "a", Pairs: []Pair{{A: 0, B: 1, Weight: 1}}}}
	p := NewProblem(testSlots(), units, students, nil, DefaultWeights())

	st, _ := Solve(p, fastOpts(), true)
	if st.SlotOf[0] < 0 || st.SlotOf[1] < 0 {
		t.Fatalf("exams not placed: %v", st.SlotOf)
	}
	if st.SlotOf[0] == st.SlotOf[1] {
		t.Errorf("warm start left a hard same-slot violation: %v", st.SlotOf)
	}
	if vs := p.Registry().HardViolations(st); len(vs) != 0 {
		t.Errorf("unexpected hard violations: %+v", vs)
	}
}

func TestRegistryDescribes(t *testing.T) {
	p := NewProblem(testSlots(), []Unit{{ID: 1, Seats: 1}}, nil, nil, DefaultWeights())
	infos := p.Registry().Describe()
	if len(infos) == 0 {
		t.Fatal("no constraints described")
	}
	var hard, soft int
	for _, i := range infos {
		switch i.Kind {
		case optimize.KindHard:
			hard++
		case optimize.KindSoft:
			soft++
		}
		if i.Name == "" || i.Title == "" || i.Description == "" {
			t.Errorf("constraint info incomplete: %+v", i)
		}
	}
	if hard == 0 || soft == 0 {
		t.Errorf("expected both hard and soft constraints, got hard=%d soft=%d", hard, soft)
	}
}

func fullCost(st *State) float64 {
	s, _ := spreadCost(st)
	a, _ := attractCost(st)
	l, _ := slotLoadCost(st)
	h, _ := holeCost(st)
	f, _ := tbauFillCost(st)
	u, _ := unplacedCost(st)
	return s + a + l + h + f + u
}

func TestIncrementalMatchesFull(t *testing.T) {
	units := []Unit{
		{ID: 1, Ancodes: []int{1}, Seats: 10},
		{ID: 2, Ancodes: []int{2}, Seats: 10},
		{ID: 3, Ancodes: []int{3}, Seats: 10},
		{ID: 4, Ancodes: []int{4}, Seats: 10},
	}
	students := []Student{
		{ID: "a", Pairs: []Pair{{A: 0, B: 1, Weight: 1}, {A: 1, B: 2, Weight: 1}, {A: 0, B: 2, Weight: 1}}},
		{ID: "b", Pairs: []Pair{{A: 2, B: 3, Weight: 1}, {A: 1, B: 3, Weight: 1}}},
	}
	attract := []AttractPair{{A: 0, B: 3, Weight: 1}}
	p := NewProblem(testSlots(), units, students, attract, DefaultWeights())
	st, _ := Solve(p, fastOpts(), false)
	inc := st.Cost()
	full := fullCost(st)
	if diff := inc - full; diff > 1e-6 || diff < -1e-6 {
		t.Errorf("incremental cost %.4f != full recompute %.4f (diff %.6f)", inc, full, diff)
	}
}

func TestAcceptedWeightZeroStillHardSameSlot(t *testing.T) {
	units := []Unit{{ID: 1, Ancodes: []int{1}, Seats: 10}, {ID: 2, Ancodes: []int{2}, Seats: 10}}
	// per-student acceptance is modelled as weight 0: no proximity penalty, but the
	// pair still forbids the same slot (it stays in hardConf).
	students := []Student{{ID: "a", Pairs: []Pair{{A: 0, B: 1, Weight: 0}}}}
	p := NewProblem(testSlots(), units, students, nil, DefaultWeights())
	st, _ := Solve(p, fastOpts(), false)
	if st.SlotOf[0] == st.SlotOf[1] {
		t.Errorf("weight-0 pair must still not share a slot: %v", st.SlotOf)
	}
	if st.spreadTotal != 0 {
		t.Errorf("weight-0 pair should contribute 0 spread, got %.4f", st.spreadTotal)
	}
}

func TestClosenessUsesRealHoursAcrossDays(t *testing.T) {
	base := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC) // Mon
	mk := func(day, slot, hour int) Slot {
		d := base.AddDate(0, 0, day-1).Add(time.Duration(hour) * time.Hour)
		return Slot{SlotRef: SlotRef{Day: day, Slot: slot, Start: d}, Seats: 100}
	}
	slots := []Slot{
		mk(1, 1, 8),  // idx0 Mon 08:00
		mk(1, 2, 16), // idx1 Mon 16:00
		mk(2, 1, 8),  // idx2 Tue 08:00
		mk(2, 2, 16), // idx3 Tue 16:00
	}
	p := NewProblem(slots, []Unit{{ID: 1}, {ID: 2}}, nil, nil, DefaultWeights())
	overnightShort := p.closeness(1, 2) // Mon 16:00 -> Tue 08:00 = 16h
	overnightLong := p.closeness(0, 3)  // Mon 08:00 -> Tue 16:00 = 32h
	if !(overnightShort > overnightLong) {
		t.Errorf("16h gap should cost more than 32h gap: %.1f vs %.1f", overnightShort, overnightLong)
	}
	if overnightShort >= p.W.SameDay {
		t.Errorf("across-day should be cheaper than same-day: %.1f >= %.1f", overnightShort, p.W.SameDay)
	}
}

// TestHoleDrivesEmptySlotToDayEdge reproduces the reported case: a day where one slot
// must stay empty and the LAST slot is occupied by a fixed SEB exam (must stay in the
// T-building). The free slot must end up at the day edge (the first slot) rather than
// between occupied slots, which is better for invigilation planning.
func TestHoleDrivesEmptySlotToDayEdge(t *testing.T) {
	slots := oneDaySlots(5) // one day, slots 1..5
	units := []Unit{
		{ID: 1, Ancodes: []int{1}, Seats: 80, Allowed: []int{0, 1, 2, 3}},
		{ID: 2, Ancodes: []int{2}, Seats: 80, Allowed: []int{0, 1, 2, 3}},
		{ID: 3, Ancodes: []int{3}, Seats: 80, Allowed: []int{0, 1, 2, 3}},
		{ID: 9, Ancodes: []int{9}, Seats: 50, Seb: true, Fixed: true, FixedSlot: 4}, // SEB pinned to the last slot
	}
	p := NewProblem(slots, units, nil, nil, DefaultWeights())
	st, _ := Solve(p, fastOpts(), false)

	if d := st.Diagnostics(); d.InteriorHoles != 0 {
		t.Errorf("free slot should sit at the day edge (0 interior holes), got %d; slots=%v", d.InteriorHoles, st.SlotOf)
	}
	if st.slotSeats[0] != 0 {
		t.Errorf("the first slot should be the empty one (SEB fixed in the last), got seats=%d; slots=%v", st.slotSeats[0], st.SlotOf)
	}
	if vs := p.Registry().HardViolations(st); len(vs) != 0 {
		t.Errorf("unexpected hard violations: %+v", vs)
	}
}

// TestHoleFillsInteriorGap: with two fixed exams at the first and last slot of a day and
// a small, conflict-free exam free to go anywhere, the small exam is pulled into the
// middle slot so no free slot remains between occupied ones.
func TestHoleFillsInteriorGap(t *testing.T) {
	slots := oneDaySlots(3)
	units := []Unit{
		{ID: 1, Ancodes: []int{1}, Seats: 50, Fixed: true, FixedSlot: 0},
		{ID: 2, Ancodes: []int{2}, Seats: 50, Fixed: true, FixedSlot: 2},
		{ID: 3, Ancodes: []int{3}, Seats: 20, Allowed: []int{0, 1, 2}}, // small, no conflicts
	}
	w := DefaultWeights()
	w.SlotLoad = 0 // isolate: only the hole term should decide where the small exam lands
	p := NewProblem(slots, units, nil, nil, w)
	st, _ := Solve(p, fastOpts(), false)

	if st.SlotOf[2] != 1 {
		t.Errorf("small exam should fill the interior gap (idx1) to avoid a hole, got %d", st.SlotOf[2])
	}
	if d := st.Diagnostics(); d.InteriorHoles != 0 {
		t.Errorf("expected no interior holes after filling, got %d", d.InteriorHoles)
	}
}

// TestHoleIncrementalMatchesFull exercises the incremental hole bookkeeping (moveUnit /
// undo) against a from-scratch recompute over many random moves on a 4-slot day where
// interior holes actually occur.
func TestHoleIncrementalMatchesFull(t *testing.T) {
	slots := oneDaySlots(4)
	units := []Unit{
		{ID: 1, Ancodes: []int{1}, Seats: 30},
		{ID: 2, Ancodes: []int{2}, Seats: 30},
		{ID: 3, Ancodes: []int{3}, Seats: 30},
		{ID: 4, Ancodes: []int{4}, Seats: 30},
	}
	students := []Student{{ID: "a", Pairs: []Pair{{A: 0, B: 1, Weight: 1}, {A: 2, B: 3, Weight: 1}}}}
	p := NewProblem(slots, units, students, nil, DefaultWeights())
	st := construct(p)
	rng := rand.New(rand.NewSource(7))
	check := func(when string, i int) {
		want, _ := holeCost(st)
		if diff := st.holeTotal - want; diff > 1e-6 || diff < -1e-6 {
			t.Fatalf("holeTotal %.4f != recompute %.4f %s move %d; slots=%v", st.holeTotal, want, when, i, st.SlotOf)
		}
	}
	for i := 0; i < 3000; i++ {
		undo := st.Propose(rng)
		if undo == nil {
			continue
		}
		check("after", i)
		if i%3 == 0 {
			undo()
			check("after undo", i)
		}
	}
}

// TestHoleTreatsForeignOnlySlotAsFree: a mid-day slot occupied ONLY by a foreign
// (not-planned-by-me) exam counts as free for our planning. The small own exam must be
// pulled into it so the day has no interior gap in OUR exams, and SlotsUsed must count
// own-occupied slots only.
func TestHoleTreatsForeignOnlySlotAsFree(t *testing.T) {
	slots := oneDaySlots(3)
	units := []Unit{
		{ID: 1, Ancodes: []int{1}, Seats: 50, Fixed: true, FixedSlot: 0},                    // own, slot 1
		{ID: 2, Ancodes: []int{2}, Seats: 50, Fixed: true, FixedSlot: 2},                    // own, slot 3
		{ID: 435, Ancodes: []int{435}, Seats: 40, Fixed: true, FixedSlot: 1, Foreign: true}, // foreign, slot 2
		{ID: 3, Ancodes: []int{3}, Seats: 20, Allowed: []int{0, 1, 2}},                      // small own, movable
	}
	w := DefaultWeights()
	w.SlotLoad = 0 // isolate the hole term
	p := NewProblem(slots, units, nil, nil, w)
	st, _ := Solve(p, fastOpts(), false)

	if st.SlotOf[3] != 1 {
		t.Errorf("small own exam should fill the foreign-only middle slot (idx1), got %d", st.SlotOf[3])
	}
	if d := st.Diagnostics(); d.InteriorHoles != 0 {
		t.Errorf("expected no interior holes once our exam fills the middle, got %d", d.InteriorHoles)
	}
}

// TestSlotsUsedCountsOwnOnly: a slot holding only a foreign exam is not counted as used.
func TestSlotsUsedCountsOwnOnly(t *testing.T) {
	slots := oneDaySlots(3)
	units := []Unit{
		{ID: 1, Ancodes: []int{1}, Seats: 50, Fixed: true, FixedSlot: 0},                    // own, slot 1
		{ID: 414, Ancodes: []int{414}, Seats: 40, Fixed: true, FixedSlot: 1, Foreign: true}, // foreign, slot 2
	}
	p := NewProblem(slots, units, nil, nil, DefaultWeights())
	st := newState(p)
	st.initCost()
	if d := st.Diagnostics(); d.SlotsUsed != 1 {
		t.Errorf("SlotsUsed should count only slots with our exams (1), got %d", d.SlotsUsed)
	}
	// the own exam is alone at the day edge → foreign-only middle slot is not an interior hole
	if d := st.Diagnostics(); d.InteriorHoles != 0 {
		t.Errorf("a single own exam at the edge is not a hole, got %d", d.InteriorHoles)
	}
}

// TestHoleMultiDayGroupingAndIncremental checks the per-day grouping and the incremental
// hole bookkeeping across MORE than one day (a hole on day 2 must not be affected by
// day 1's occupancy, and moves that cross days must keep holeTotal correct).
func TestHoleMultiDayGroupingAndIncremental(t *testing.T) {
	t0 := time.Date(2026, 7, 6, 8, 30, 0, 0, time.UTC)
	mk := func(day, slot, addH int) Slot {
		return Slot{SlotRef: SlotRef{Day: day, Slot: slot, Start: t0.Add(time.Duration(addH) * time.Hour)}, Seats: 1000}
	}
	// day 1: slots 1..3 (idx 0..2); day 2: slots 1..4 (idx 3..6, on the next calendar day)
	slots := []Slot{
		mk(1, 1, 0), mk(1, 2, 2), mk(1, 3, 4),
		mk(2, 1, 24), mk(2, 2, 26), mk(2, 3, 28), mk(2, 4, 30),
	}
	// dayOfSlot must group the two days correctly
	p := NewProblem(slots, []Unit{{ID: 1, Seats: 1}}, nil, nil, DefaultWeights())
	wantDay := []int{0, 0, 0, 1, 1, 1, 1}
	for s, w := range wantDay {
		if p.dayOfSlot[s] != w {
			t.Fatalf("dayOfSlot[%d]=%d, want %d", s, p.dayOfSlot[s], w)
		}
	}
	if len(p.days) != 2 || len(p.days[0]) != 3 || len(p.days[1]) != 4 {
		t.Fatalf("day grouping wrong: %v", p.days)
	}

	units := []Unit{
		{ID: 1, Ancodes: []int{1}, Seats: 20},
		{ID: 2, Ancodes: []int{2}, Seats: 20},
		{ID: 3, Ancodes: []int{3}, Seats: 20},
		{ID: 4, Ancodes: []int{4}, Seats: 20},
		{ID: 5, Ancodes: []int{5}, Seats: 20},
	}
	students := []Student{{ID: "a", Pairs: []Pair{{A: 0, B: 1, Weight: 1}, {A: 2, B: 3, Weight: 1}}}}
	p2 := NewProblem(slots, units, students, nil, DefaultWeights())
	st := construct(p2)
	rng := rand.New(rand.NewSource(11))
	for i := 0; i < 4000; i++ {
		undo := st.Propose(rng)
		if undo == nil {
			continue
		}
		want, _ := holeCost(st)
		if diff := st.holeTotal - want; diff > 1e-6 || diff < -1e-6 {
			t.Fatalf("holeTotal %.4f != recompute %.4f after move %d; slots=%v", st.holeTotal, want, i, st.SlotOf)
		}
		if i%4 == 0 {
			undo()
			want2, _ := holeCost(st)
			if diff := st.holeTotal - want2; diff > 1e-6 || diff < -1e-6 {
				t.Fatalf("holeTotal %.4f != recompute %.4f after undo %d", st.holeTotal, want2, i)
			}
		}
	}
}

func TestCrossCampusSameDayPenalty(t *testing.T) {
	build := func(cross bool) float64 {
		units := []Unit{{ID: 1, Seats: 10}, {ID: 2, Seats: 10}}
		if cross {
			units[0].Location = "Campus Pasing"
		}
		students := []Student{{ID: "a", Pairs: []Pair{{A: 0, B: 1, Weight: 1, CrossLoc: cross}}}}
		p := NewProblem(testSlots(), units, students, nil, DefaultWeights())
		st := newState(p)
		st.setPhysical(0, 0) // day1 slot1
		st.setPhysical(1, 1) // day1 slot2 (same day)
		st.initCost()
		return st.spreadTotal
	}
	cross := build(true)
	same := build(false)
	if cross-same < DefaultWeights().CrossCampus {
		t.Errorf("cross-campus same-day should add at least CrossCampus (%.0f): cross=%.0f same=%.0f",
			DefaultWeights().CrossCampus, cross, same)
	}
}
