package roomplan

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/obcode/plexams.go/plexams/optimize"
)

// --- test fixtures ---

var berlin = mustLoad("Europe/Berlin")

func mustLoad(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

func at(day, hour int) time.Time { return time.Date(2026, 7, day, hour, 0, 0, 0, berlin) }

// buildScenario builds a small but structurally rich problem: own rooms on floors 0/1/2, a
// booked EXaHM/SEB T-room, three slots on one day, exams that must split across rooms,
// share a slot, need an EXaHM room, and one NTA-alone seat.
func buildScenario(summer bool) *Problem {
	rooms := []Room{
		{Name: "R0.001", Seats: 4, OwnRoom: true, HeatLevel: 0},                          // 0: floor 0
		{Name: "R1.001", Seats: 4, OwnRoom: true, HeatLevel: 1},                          // 1: floor 1
		{Name: "R2.001", Seats: 3, OwnRoom: true, HeatLevel: 2},                          // 2: floor 2
		{Name: "T3.001", Seats: 4, OwnRoom: false, HeatLevel: 0, Exahm: true, Seb: true}, // 3: booked
	}
	slots := []Slot{{Start: at(6, 9)}, {Start: at(6, 12)}, {Start: at(6, 15)}} // one day, 3 slots
	ownRooms := []int{0, 1, 2}
	exams := []Exam{
		// A: 6 normal students in slot 0 → needs ≥2 own rooms.
		{Ancode: 100, Slot: 0, NormalCount: 6, AllowedNormal: ownRooms, AllowedAlone: ownRooms},
		// B: 3 normal in slot 0 (shares the slot with A).
		{Ancode: 200, Slot: 0, NormalCount: 3, AllowedNormal: ownRooms, AllowedAlone: ownRooms},
		// C: 2 normal in slot 1.
		{Ancode: 300, Slot: 1, NormalCount: 2, AllowedNormal: ownRooms, AllowedAlone: ownRooms},
		// D: 3 normal EXaHM in slot 2 → only the T-room.
		{Ancode: 400, Slot: 2, Exahm: true, NormalCount: 3, AllowedNormal: []int{3}, AllowedAlone: []int{3}},
	}
	var seats []Seat
	add := func(exam, n int, kind SeatKind) {
		for k := 0; k < n; k++ {
			seats = append(seats, Seat{Exam: exam, Mtknr: mtk(exam, k), Kind: kind})
		}
	}
	add(0, 6, Normal)
	add(1, 3, Normal)
	// C: 2 normal + 1 NTA-alone (so NormalCount stays 2, alone seat is extra).
	add(2, 2, Normal)
	seats = append(seats, Seat{Exam: 2, Mtknr: "NTA-300", Kind: NTAAlone})
	add(3, 3, Normal)

	w := DefaultWeights()
	p := NewProblem(slots, rooms, exams, seats, w)
	p.Summer = summer
	return p
}

func mtk(exam, k int) string { return string(rune('A'+exam)) + string(rune('0'+k)) }

// recomputeSoftCost sums the Registry soft-constraint costs (the authoritative full
// recompute) for comparison with the incrementally maintained State.Cost().
func recomputeSoftCost(p *Problem, st *State) float64 {
	var total float64
	for _, c := range p.Registry().Soft {
		pen, _ := c.Cost(st)
		total += pen
	}
	return total
}

// --- tests ---

func TestFloorFromName(t *testing.T) {
	cases := map[string]int{
		"R2.007": 2, "R0.011": 0, "R1.046": 1, "R3.026": 3,
		"T3.015": 0, "ONLINE_1": 0, "E0.103": 0, "": 0, "R": 0, "RX.001": 0,
	}
	for name, want := range cases {
		if got := FloorFromName(name); got != want {
			t.Errorf("FloorFromName(%q) = %d, want %d", name, got, want)
		}
	}
}

func TestConstructIsHardFeasible(t *testing.T) {
	for _, summer := range []bool{false, true} {
		p := buildScenario(summer)
		st := construct(p)
		if vs := p.Registry().HardViolations(st); len(vs) > 0 {
			t.Fatalf("summer=%v: construct produced %d hard violations: %+v", summer, len(vs), vs)
		}
	}
}

// TestMovesStayFeasibleAndCostConsistent is the core invariant: after every accepted
// Propose move the state remains hard-feasible, and the incrementally maintained Cost()
// equals the full recompute; undo restores both exactly.
func TestMovesStayFeasibleAndCostConsistent(t *testing.T) {
	p := buildScenario(true)
	st := construct(p)
	rng := rand.New(rand.NewSource(42))
	reg := p.Registry()

	for it := 0; it < 20000; it++ {
		before := st.Cost()
		undo := st.Propose(rng)
		if undo == nil {
			continue
		}
		// feasibility must be preserved on every visited state
		if vs := reg.HardViolations(st); len(vs) > 0 {
			t.Fatalf("iter %d: move produced hard violations: %+v", it, vs)
		}
		// incremental cost must match the full recompute
		if got, want := st.Cost(), recomputeSoftCost(p, st); math.Abs(got-want) > 1e-6 {
			t.Fatalf("iter %d: incremental cost %.4f != recompute %.4f", it, got, want)
		}
		// randomly undo and check the pre-move cost is restored exactly
		if rng.Float64() < 0.5 {
			undo()
			if got := st.Cost(); math.Abs(got-before) > 1e-6 {
				t.Fatalf("iter %d: undo cost %.4f != before %.4f", it, got, before)
			}
			if vs := reg.HardViolations(st); len(vs) > 0 {
				t.Fatalf("iter %d: undo produced hard violations: %+v", it, vs)
			}
		}
	}
}

func TestSolvePlacesEveryoneAndValidates(t *testing.T) {
	p := buildScenario(false)
	opts := optimize.DefaultOptions()
	opts.Iterations = 50000
	opts.Seed = 7
	st, _ := Solve(p, opts, false)

	if n := st.UnplacedCount(); n != 0 {
		t.Fatalf("expected everyone placed, got %d unplaced", n)
	}
	if vs := p.Registry().HardViolations(st); len(vs) > 0 {
		t.Fatalf("solved plan has hard violations: %+v", vs)
	}
	// every exam's seats are assigned; the EXaHM exam sits in the T-room only.
	for _, a := range st.Assignments() {
		if a.Ancode == 400 && a.Room != "T3.001" {
			t.Errorf("EXaHM exam 400 in non-EXaHM room %s", a.Room)
		}
	}
}

// TestSummerCooldown: an own room must never be used in two directly consecutive slots.
func TestSummerCooldown(t *testing.T) {
	// two exams, each 3 students, only room R0.001 allowed, in consecutive slots. In summer
	// they cannot both use it → the second exam's seats stay unplaced.
	rooms := []Room{{Name: "R0.001", Seats: 4, OwnRoom: true}}
	slots := []Slot{{Start: at(6, 9)}, {Start: at(6, 12)}}
	exams := []Exam{
		{Ancode: 1, Slot: 0, NormalCount: 3, AllowedNormal: []int{0}, AllowedAlone: []int{0}},
		{Ancode: 2, Slot: 1, NormalCount: 3, AllowedNormal: []int{0}, AllowedAlone: []int{0}},
	}
	var seats []Seat
	for e := 0; e < 2; e++ {
		for k := 0; k < 3; k++ {
			seats = append(seats, Seat{Exam: e, Mtknr: mtk(e, k), Kind: Normal})
		}
	}
	p := NewProblem(slots, rooms, exams, seats, DefaultWeights())
	p.Summer = true

	opts := optimize.DefaultOptions()
	opts.Iterations = 20000
	st, _ := Solve(p, opts, false)

	if vs := (summerCooldownC{}).Check(st); len(vs) > 0 {
		t.Fatalf("cooldown violated: %+v", vs)
	}
	// exactly one exam gets the room; the other is fully unplaced (3 seats).
	if st.UnplacedCount() != 3 {
		t.Errorf("expected 3 unplaced (one exam blocked by cooldown), got %d", st.UnplacedCount())
	}

	// without summer, the same rooms can be reused in consecutive slots → all placed.
	p2 := NewProblem(slots, rooms, exams, seats, DefaultWeights())
	st2, _ := Solve(p2, opts, false)
	if st2.UnplacedCount() != 0 {
		t.Errorf("winter: expected all placed, got %d unplaced", st2.UnplacedCount())
	}
}

// TestHeatFloorPrefersLowerFloorLate: in summer a late exam prefers a lower floor.
func TestHeatFloorPrefersLowerFloor(t *testing.T) {
	// one exam of 3 students late in the day, choice between a floor-0 and a floor-2 room
	// (both fit). The heat term should drive it to the floor-0 room.
	rooms := []Room{
		{Name: "R0.001", Seats: 3, OwnRoom: true, HeatLevel: 0},
		{Name: "R2.001", Seats: 3, OwnRoom: true, HeatLevel: 2},
	}
	slots := []Slot{{Start: at(6, 16)}} // late
	exams := []Exam{{Ancode: 1, Slot: 0, NormalCount: 3, AllowedNormal: []int{0, 1}, AllowedAlone: []int{0, 1}}}
	var seats []Seat
	for k := 0; k < 3; k++ {
		seats = append(seats, Seat{Exam: 0, Mtknr: mtk(0, k), Kind: Normal})
	}
	p := NewProblem(slots, rooms, exams, seats, DefaultWeights())
	p.Summer = true
	opts := optimize.DefaultOptions()
	opts.Iterations = 20000
	opts.Seed = 3
	st, _ := Solve(p, opts, false)
	for _, a := range st.Assignments() {
		if a.Room != "R0.001" {
			t.Errorf("late summer exam placed on hot floor: room %s", a.Room)
		}
	}
}

// TestPrePlannedFixed: a fixed seat stays in its room through solving.
func TestPrePlannedFixed(t *testing.T) {
	rooms := []Room{{Name: "R0.001", Seats: 5, OwnRoom: true}, {Name: "R1.001", Seats: 5, OwnRoom: true}}
	slots := []Slot{{Start: at(6, 9)}}
	exams := []Exam{{Ancode: 1, Slot: 0, NormalCount: 3, AllowedNormal: []int{0, 1}, AllowedAlone: []int{0, 1}}}
	seats := []Seat{
		{Exam: 0, Mtknr: "X0", Kind: Normal, Fixed: true, FixedRoom: 1}, // pinned to R1.001
		{Exam: 0, Mtknr: "X1", Kind: Normal},
		{Exam: 0, Mtknr: "X2", Kind: Normal},
	}
	p := NewProblem(slots, rooms, exams, seats, DefaultWeights())
	opts := optimize.DefaultOptions()
	opts.Iterations = 10000
	st, _ := Solve(p, opts, false)
	if st.roomOf[0] != 1 {
		t.Fatalf("fixed seat moved to room %d, want 1", st.roomOf[0])
	}
	if vs := (prePlannedC{}).Check(st); len(vs) > 0 {
		t.Fatalf("pre-planned violation: %+v", vs)
	}
}

// TestRoomTurnaround: two exams in the same single room in slots too close together (a long
// exam overrunning into the next slot) cannot both use it; a wide enough gap is fine.
func TestRoomTurnaround(t *testing.T) {
	rooms := []Room{{Name: "R0.001", Seats: 5, OwnRoom: true}}
	exams := []Exam{
		// exam 1 at 09:00 runs 120 min → ends 11:00; +15 turnaround → room free 11:15.
		{Ancode: 1, Slot: 0, Duration: 120, NormalCount: 3, AllowedNormal: []int{0}, AllowedAlone: []int{0}},
		// exam 2 at 11:00 would need the room from 11:00 — too soon (needs 11:15).
		{Ancode: 2, Slot: 1, Duration: 60, NormalCount: 3, AllowedNormal: []int{0}, AllowedAlone: []int{0}},
	}
	seats := []Seat{
		{Exam: 0, Mtknr: "a", Kind: Normal}, {Exam: 0, Mtknr: "b", Kind: Normal}, {Exam: 0, Mtknr: "c", Kind: Normal},
		{Exam: 1, Mtknr: "d", Kind: Normal}, {Exam: 1, Mtknr: "e", Kind: Normal}, {Exam: 1, Mtknr: "f", Kind: Normal},
	}
	slotsClose := []Slot{{Start: at(6, 9)}, {Start: at(6, 11)}} // 09:00 & 11:00 — too close
	slotsFar := []Slot{{Start: at(6, 9)}, {Start: at(6, 13)}}   // 09:00 & 13:00 — fine

	opts := optimize.DefaultOptions()
	opts.Iterations = 20000

	pClose := NewProblem(slotsClose, rooms, exams, seats, DefaultWeights())
	stClose, _ := Solve(pClose, opts, false)
	if vs := (overrunC{}).Check(stClose); len(vs) > 0 {
		t.Fatalf("turnaround violated: %+v", vs)
	}
	if stClose.UnplacedCount() != 3 { // one exam blocked out of the shared room
		t.Errorf("close slots: expected 3 unplaced (turnaround), got %d", stClose.UnplacedCount())
	}

	pFar := NewProblem(slotsFar, rooms, exams, seats, DefaultWeights())
	stFar, _ := Solve(pFar, opts, false)
	if stFar.UnplacedCount() != 0 {
		t.Errorf("far slots: expected all placed, got %d unplaced", stFar.UnplacedCount())
	}
}

// TestRoomPreferences: SEB exams prefer plain SEB rooms (R-Bau) over booked EXaHM rooms, and
// EXaHM exams prefer a booked EXaHM room over an own (R-Bau) fallback EXaHM room (e.g. R1.011).
func TestRoomPreferences(t *testing.T) {
	rooms := []Room{
		{Name: "T3.001", Seats: 4, OwnRoom: false, Exahm: true, Seb: true}, // 0: booked EXaHM/SEB
		{Name: "R1.009", Seats: 4, OwnRoom: true, Seb: true},               // 1: R-Bau SEB (own)
		{Name: "R1.011", Seats: 1, OwnRoom: true, Exahm: true},             // 2: own EXaHM fallback
	}
	slots := []Slot{{Start: at(6, 9)}}
	exams := []Exam{
		{Ancode: 1, Slot: 0, Seb: true, NormalCount: 3, AllowedNormal: []int{0, 1}, AllowedAlone: []int{0, 1}},   // SEB
		{Ancode: 2, Slot: 0, Exahm: true, NormalCount: 1, AllowedNormal: []int{0, 2}, AllowedAlone: []int{0, 2}}, // EXaHM
	}
	seats := []Seat{
		{Exam: 0, Mtknr: "s0", Kind: Normal}, {Exam: 0, Mtknr: "s1", Kind: Normal}, {Exam: 0, Mtknr: "s2", Kind: Normal},
		{Exam: 1, Mtknr: "e0", Kind: Normal},
	}
	p := NewProblem(slots, rooms, exams, seats, DefaultWeights())
	opts := optimize.DefaultOptions()
	opts.Iterations = 20000
	opts.Seed = 5
	st, _ := Solve(p, opts, false)

	for _, a := range st.Assignments() {
		if a.Ancode == 1 && a.Room == "T3.001" {
			t.Errorf("SEB exam went into the booked EXaHM room instead of R-Bau: %s", a.Room)
		}
		if a.Ancode == 2 && a.Room != "T3.001" {
			t.Errorf("EXaHM exam used the own fallback room %s instead of the booked T3.001", a.Room)
		}
	}
	// incremental cost must still equal the full recompute with the preference terms active.
	if got, want := st.Cost(), recomputeSoftCost(p, st); math.Abs(got-want) > 1e-6 {
		t.Fatalf("incremental cost %.4f != recompute %.4f", got, want)
	}
}

func TestRegistryDescribeCoversAll(t *testing.T) {
	p := buildScenario(true)
	infos := p.Registry().Describe()
	if len(infos) != 15 { // 7 hard + 8 soft
		t.Fatalf("expected 15 constraints, got %d", len(infos))
	}
	seen := map[string]bool{}
	for _, in := range infos {
		if in.Name == "" || in.Title == "" || in.Description == "" {
			t.Errorf("constraint %+v missing name/title/description", in)
		}
		if seen[in.Name] {
			t.Errorf("duplicate constraint name %q", in.Name)
		}
		seen[in.Name] = true
	}
}
