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
