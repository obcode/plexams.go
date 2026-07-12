package examplan

import (
	"testing"
	"time"
)

func TestMarginalOverflow(t *testing.T) {
	cases := []struct{ before, add, cap, want int }{
		{0, 80, 100, 0},    // fits under capacity
		{0, 80, 0, 80},     // no booking → full overflow
		{50, 80, 100, 30},  // 50 already there, 80 more, cap 100 → 30 over
		{120, 20, 100, 20}, // already over → all new seats overflow
	}
	for _, c := range cases {
		if got := marginalOverflow(c.before, c.add, c.cap); got != c.want {
			t.Errorf("marginalOverflow(%d,%d,%d) = %d, want %d", c.before, c.add, c.cap, got, c.want)
		}
	}
}

func TestOverflowPenaltyPerSeatBeyondBooking(t *testing.T) {
	p := &Problem{
		Slots: []Slot{{SlotRef: SlotRef{Start: time.Date(2026, 7, 6, 8, 30, 0, 0, time.UTC)}, SebSeats: 80, ExahmSeats: 0}},
		W:     Weights{OverflowSeat: 10},
	}
	// 100 SEB placed, 80 booked → 20 overflow × 10 = 200
	if got := p.overflowPenalty(0, 0, 100); got != 200 {
		t.Errorf("overflowPenalty = %v, want 200", got)
	}
	// within booking → 0
	if got := p.overflowPenalty(0, 0, 80); got != 0 {
		t.Errorf("overflowPenalty within booking = %v, want 0", got)
	}
	// off (weight 0) → 0
	p.W.OverflowSeat = 0
	if got := p.overflowPenalty(0, 0, 100); got != 0 {
		t.Errorf("overflowPenalty off = %v, want 0", got)
	}
}

// A big and a small SEB exam that a student shares (so they cannot share a slot) compete
// for a single SEB booking (80 seats). The big one must take the booking; the small one
// overflows to the no-booking slot — because per-seat overflow makes spilling the big exam
// far costlier than the small one.
func TestPhaseAKeepsBigExamInBooking(t *testing.T) {
	t0 := time.Date(2026, 7, 6, 8, 30, 0, 0, time.UTC)
	slots := []Slot{
		{SlotRef: SlotRef{Start: t0}, Seats: 1000, SebSeats: 80},                   // idx0: booked SEB=80
		{SlotRef: SlotRef{Start: t0.Add(2 * time.Hour)}, Seats: 1000, SebSeats: 0}, // idx1: no booking
	}
	units := []Unit{
		{ID: 1, Ancodes: []int{1}, Seats: 80, Seb: true},
		{ID: 2, Ancodes: []int{2}, Seats: 20, Seb: true},
	}
	students := []Student{{ID: "s1", Pairs: []Pair{{A: 0, B: 1, Weight: 1}}}}
	w := DefaultWeights()
	w.TbauFill = 10000
	w.OverflowSeat = 10000
	w.SlotLoad = 0
	w.Hole = 0
	p := NewProblem(slots, units, students, nil, w)
	// same-start separation only (they just may not share a slot)
	p.SetHardSeparations(map[[2]int]int{{0, 1}: 1, {1, 0}: 1})

	st, _ := Solve(p, fastOpts(), false)
	if st.SlotOf[0] != 0 {
		t.Errorf("big SEB exam should take the booking (slot 0), got slot %d", st.SlotOf[0])
	}
	if st.SlotOf[1] != 1 {
		t.Errorf("small SEB exam should overflow (slot 1), got slot %d", st.SlotOf[1])
	}
}
