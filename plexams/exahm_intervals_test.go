package plexams

import (
	"testing"
	"time"

	"github.com/obcode/plexams.go/graph/model"
)

func min2(v int) *int { return &v }

func TestExahmRoomBuffers(t *testing.T) {
	tests := []struct {
		name              string
		constraints       *model.Constraints
		wantPre, wantPost time.Duration
	}{
		{"nil → default 30/30", nil, exahmDefaultBuffer, exahmDefaultBuffer},
		{"no room constraints → default", &model.Constraints{}, exahmDefaultBuffer, exahmDefaultBuffer},
		{
			"override widens to 60/60 (Embedded Computing)",
			&model.Constraints{RoomConstraints: &model.RoomConstraints{PreExamMinutes: min2(60), PostExamMinutes: min2(60)}},
			60 * time.Minute, 60 * time.Minute,
		},
		{
			"override may shorten to 15/15",
			&model.Constraints{RoomConstraints: &model.RoomConstraints{PreExamMinutes: min2(15), PostExamMinutes: min2(15)}},
			15 * time.Minute, 15 * time.Minute,
		},
		{
			"zero override ignored → default",
			&model.Constraints{RoomConstraints: &model.RoomConstraints{PreExamMinutes: min2(0), PostExamMinutes: min2(0)}},
			exahmDefaultBuffer, exahmDefaultBuffer,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pre, post := exahmRoomBuffers(tt.constraints)
			if pre != tt.wantPre || post != tt.wantPost {
				t.Errorf("exahmRoomBuffers() = (%v, %v), want (%v, %v)", pre, post, tt.wantPre, tt.wantPost)
			}
		})
	}
}

// day builds a local time for 2026-07-21 at hh:mm (the Embedded Computing day).
func day(hh, mm int) time.Time {
	return time.Date(2026, 7, 21, hh, mm, 0, 0, time.Local)
}

func TestExahmWindowCovered(t *testing.T) {
	// booking of one EXaHM room 16:00–18:30 (the Anny booking the user made for Embedded).
	small := []bookedRoomInterval{{from: day(16, 0), until: day(18, 30), exahm: true, seats: 30}}
	// a larger booking 14:00–18:30 (fits two 90-min exams back to back).
	large := []bookedRoomInterval{{from: day(14, 0), until: day(18, 30), exahm: true, seats: 30}}

	mins := func(m int) time.Duration { return time.Duration(m) * time.Minute }

	tests := []struct {
		name           string
		intervals      []bookedRoomInterval
		exahm          bool
		start          time.Time
		dur, pre, post time.Duration
		want           bool
	}{
		{
			// Embedded Computing: 120 min + 60/60 buffer needs [15:30, 19:30]; the 16:00–18:30
			// booking is far too small → must be rejected (the reported bug).
			"embedded computing 16:30 does not fit 16:00-18:30",
			small, true, day(16, 30), mins(120), mins(60), mins(60), false,
		},
		{
			// Even with the default 30/30 buffer a 120-min exam needs [16:00, 19:00] → the
			// booking ends 18:30, still too small.
			"120min default buffer 16:30 does not fit either",
			small, true, day(16, 30), mins(120), mins(30), mins(30), false,
		},
		{
			// A 90-min exam at 16:30 with 30/30 needs [16:00, 18:30] — exactly the booking.
			"90min default buffer 16:30 fits exactly",
			small, true, day(16, 30), mins(90), mins(30), mins(30), true,
		},
		{
			// Two 90-min exams in the 14:00–18:30 booking: first at 14:30 needs [14:00, 16:30].
			"first 90min exam 14:30 fits large booking",
			large, true, day(14, 30), mins(90), mins(30), mins(30), true,
		},
		{
			// second at 16:30 needs [16:00, 18:30].
			"second 90min exam 16:30 fits large booking",
			large, true, day(16, 30), mins(90), mins(30), mins(30), true,
		},
		{
			// a placement before the booking starts is never covered.
			"12:30 not covered by 16:00 booking",
			small, true, day(12, 30), mins(90), mins(30), mins(30), false,
		},
		{
			// no EXaHM-capable room booked → EXaHM exam not covered.
			"seb-only booking does not cover an exahm exam",
			[]bookedRoomInterval{{from: day(14, 0), until: day(18, 30), seb: true, seats: 30}},
			true, day(16, 30), mins(90), mins(30), mins(30), false,
		},
		{
			// a SEB exam accepts an EXaHM-capable booked room too.
			"seb exam accepts exahm booking",
			large, false, day(16, 30), mins(90), mins(30), mins(30), true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := exahmWindowCovered(tt.intervals, tt.exahm, tt.start, tt.dur, tt.pre, tt.post)
			if got != tt.want {
				t.Errorf("exahmWindowCovered() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExahmWindowSeats(t *testing.T) {
	mins := func(m int) time.Duration { return time.Duration(m) * time.Minute }

	// A small EXaHM room booked long enough to cover the window (10:00–12:30) with only 5
	// seats, plus two big rooms booked too short (10:30–11:30, 20 seats each). Only the
	// covering small room counts: a 40-student exam cannot be seated even though the block-
	// based seat total (45) looks sufficient — this is the Softwareentwicklung I case.
	mixed := []bookedRoomInterval{
		{from: day(10, 0), until: day(12, 30), exahm: true, seats: 5},
		{from: day(10, 30), until: day(11, 30), exahm: true, seats: 20},
		{from: day(10, 30), until: day(11, 30), exahm: true, seats: 20},
	}
	// exam 90 min at 10:30, default 30/30 → window [10:00, 12:30].
	if got := exahmWindowSeats(mixed, true, day(10, 30), mins(90), mins(30), mins(30)); got != 5 {
		t.Errorf("mixed window seats = %d, want 5 (only the fully-covering room counts)", got)
	}

	// all three booked to cover the window → all seats count.
	full := []bookedRoomInterval{
		{from: day(10, 0), until: day(12, 30), exahm: true, seats: 5},
		{from: day(10, 0), until: day(12, 30), exahm: true, seats: 20},
		{from: day(10, 0), until: day(12, 30), exahm: true, seats: 20},
	}
	if got := exahmWindowSeats(full, true, day(10, 30), mins(90), mins(30), mins(30)); got != 45 {
		t.Errorf("full window seats = %d, want 45", got)
	}
}

// TestExahmWindowGateNTAAbsorbedByBuffer pins the phase-A gate contract (buildExamPlanProblem):
// the EXaHM booking-window coverage is checked against the exam's BASE duration, NOT the
// NTA-extended one. An NTA student is seated in a separate NTA room booked later at room
// planning, and the modest extension is absorbed by the teardown buffer — so an NTA must not
// disqualify a slot whose booking already fits the base exam. Regression for exam 539 (90 min,
// 67 regs, +10 % NTA → 99 min): it was dropped from the only morning MUC.DAI slot on 23.7
// because its NTA window ran 9 min past the 08:00–10:30 booking. The gate must query
// exahmWindowSeats with the base duration (covered), never the NTA-extended one (uncovered).
func TestExahmWindowGateNTAAbsorbedByBuffer(t *testing.T) {
	mins := func(m int) time.Duration { return time.Duration(m) * time.Minute }
	// morning booking exactly covering a 90-min exam + 30/30 buffer at 08:30: 08:00–10:30,
	// three EXaHM rooms of 30 seats (90 seats, enough for the 67-student exam).
	booking := []bookedRoomInterval{
		{from: day(8, 0), until: day(10, 30), exahm: true, seats: 30},
		{from: day(8, 0), until: day(10, 30), exahm: true, seats: 30},
		{from: day(8, 0), until: day(10, 30), exahm: true, seats: 30},
	}
	const seatsNeeded = 67

	// base duration (90 min) → window [08:00, 10:30] exactly covered → all 90 seats count,
	// so the slot stays available. This is what the gate now uses.
	if got := exahmWindowSeats(booking, true, day(8, 30), mins(90), mins(30), mins(30)); got < seatsNeeded {
		t.Errorf("base-duration window seats = %d, want >= %d (slot must stay available)", got, seatsNeeded)
	}
	// NTA-extended duration (99 min) → window [08:00, 10:39] runs past the booking → 0 seats.
	// Using this for the gate would wrongly drop the slot — the bug being guarded against.
	if got := exahmWindowSeats(booking, true, day(8, 30), mins(99), mins(30), mins(30)); got != 0 {
		t.Errorf("NTA-extended window seats = %d, want 0 (documents why the gate must use base duration)", got)
	}
}

func TestCumulativeOverloads(t *testing.T) {
	// exam A (default 30/30, shared → 15 occupancy) at 08:30, 90 min → occ [08:15, 10:15].
	a := cumExam{id: 1, seats: 30, exahm: true, from: day(8, 15), to: day(10, 15)}
	// exam B (Embedded-like, override 60/60 → full occupancy) at 10:30, 120 min → occ
	// [09:30, 13:30]; its setup reaches back into A's teardown (overlap [09:30, 10:15]).
	b := cumExam{id: 2, seats: 30, exahm: true, from: day(9, 30), to: day(13, 30)}

	oneRoom := []bookedRoomInterval{{from: day(8, 0), until: day(14, 0), exahm: true, seats: 30}}
	twoRooms := []bookedRoomInterval{
		{from: day(8, 0), until: day(14, 0), exahm: true, seats: 30},
		{from: day(8, 0), until: day(14, 0), exahm: true, seats: 30},
	}

	// one 30-seat room: during [09:30,10:15] both need it (60 > 30) → overload.
	if ov := cumulativeOverloads([]cumExam{a, b}, oneRoom, true); len(ov) == 0 {
		t.Error("expected a cumulative overload when a long exam overlaps the previous slot in one room")
	}
	// two rooms (60 seats): 30+30 fit simultaneously → no overload.
	if ov := cumulativeOverloads([]cumExam{a, b}, twoRooms, false); len(ov) != 0 {
		t.Errorf("did not expect an overload with two rooms, got %+v", ov)
	}
	// A alone in one room → no overload.
	if ov := cumulativeOverloads([]cumExam{a}, oneRoom, false); len(ov) != 0 {
		t.Errorf("did not expect an overload for a single exam, got %+v", ov)
	}
}
