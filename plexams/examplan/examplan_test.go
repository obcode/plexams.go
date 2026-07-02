package examplan

import (
	"testing"
	"time"

	"github.com/obcode/plexams.go/plexams/optimize"
)

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

	st, _ := Solve(p, fastOpts())

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

func TestSolveRespectsExahmCapacity(t *testing.T) {
	slots := testSlots()
	slots[2].ExahmSeats = 10 // only idx2 is an EXaHM slot

	units := []Unit{
		{ID: 1, Ancodes: []int{1}, Seats: 10, Exahm: true},
		{ID: 2, Ancodes: []int{2}, Seats: 10}, // normal
	}
	p := NewProblem(slots, units, nil, nil, DefaultWeights())

	st, _ := Solve(p, fastOpts())

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

	st, _ := Solve(p, fastOpts())

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
	sa, _ := Solve(build(), fastOpts())
	sb, _ := Solve(build(), fastOpts())
	for i := range sa.SlotOf {
		if sa.SlotOf[i] != sb.SlotOf[i] {
			t.Fatalf("not deterministic at unit %d: %v vs %v", i, sa.SlotOf, sb.SlotOf)
		}
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
	u, _ := unplacedCost(st)
	return s + a + l + u
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
	st, _ := Solve(p, fastOpts())
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
	st, _ := Solve(p, fastOpts())
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
