package plexams

import (
	"strings"
	"testing"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/preplancalc"
)

// vday builds a local time on 2026-07-16 (the Softwareentwicklung I day).
func vday(hh, mm int) time.Time {
	return time.Date(2026, 7, 16, hh, mm, 0, 0, time.Local)
}

func iptr(v int) *int { return &v }

// TestValidatePreplanExahmWindowSeats reproduces the reported bug: an EXaHM exam is placed
// at 10:30 while its booked rooms only reach 11:30. The block-based seat count looks fine,
// but no booked room covers the exam's real window (10:00–12:30) with enough seats, so the
// validation must flag it as an error.
func TestValidatePreplanExahmWindowSeats(t *testing.T) {
	block := 120 * time.Minute
	start := vday(10, 30)
	exam := &model.PreplanExam{
		ID: 1, Module: "Softwareentwicklung I", ExamKind: "EXaHM",
		ExpectedStudents: 40, Duration: iptr(90), PlannedStarttime: &start,
	}
	exahmRooms := []preplancalc.RoomCapacity{{Name: "T3.023", Seats: 20}, {Name: "T3.021", Seats: 20}}
	// block-based capacity says 40 seats are booked for the slot (looks sufficient)…
	booked := map[time.Time]*slotBooking{
		start: {exahmSeats: 40, seats: 40, rooms: map[string]bool{"T3.023": true, "T3.021": true}},
	}

	t.Run("booking too short is flagged", func(t *testing.T) {
		// …but the rooms are booked only 10:30–11:30, covering neither the pre-buffer nor the
		// full run, so window seats = 0.
		intervals := []bookedRoomInterval{
			{from: vday(10, 30), until: vday(11, 30), exahm: true, seats: 20},
			{from: vday(10, 30), until: vday(11, 30), exahm: true, seats: 20},
		}
		res := validatePreplan([]*model.PreplanExam{exam}, exahmRooms, nil, booked, 24, intervals, block)
		if res.Ok {
			t.Fatal("expected validation to fail for the too-short EXaHM booking")
		}
		found := false
		for _, m := range res.Messages {
			if strings.Contains(m, "Softwareentwicklung I") && strings.Contains(m, "bieten nur") {
				found = true
			}
		}
		if !found {
			t.Errorf("expected a window-seats finding, got messages: %v", res.Messages)
		}
	})

	t.Run("over-pack: two exams need whole rooms that do not fit is flagged", func(t *testing.T) {
		// two EXaHM rooms 08:00–10:30 (30 seats each). Two exams at 08:30 (20 + 50) need three
		// whole rooms (1 + 2) but only two are booked → the plan does not pack.
		st := vday(8, 30)
		e1 := &model.PreplanExam{ID: 10, Module: "A", ExamKind: "EXaHM", ExpectedStudents: 20, Duration: iptr(90), PlannedStarttime: &st}
		e2 := &model.PreplanExam{ID: 11, Module: "B", ExamKind: "EXaHM", ExpectedStudents: 50, Duration: iptr(90), PlannedStarttime: &st}
		iv := []bookedRoomInterval{
			{room: "T3.015", from: vday(8, 0), until: vday(10, 30), exahm: true, seats: 30},
			{room: "T3.016", from: vday(8, 0), until: vday(10, 30), exahm: true, seats: 30},
		}
		bk := map[time.Time]*slotBooking{st: {exahmSeats: 60, seats: 60, rooms: map[string]bool{"T3.015": true, "T3.016": true}}}
		res := validatePreplan([]*model.PreplanExam{e1, e2}, exahmRooms, nil, bk, 24, iv, block)
		if res.Ok {
			t.Fatal("expected validation to fail: 20+50 do not pack into two 30-seat rooms")
		}
		found := false
		for _, m := range res.Messages {
			if strings.Contains(m, "passt nicht mehr in die gebuchten Räume") {
				found = true
			}
		}
		if !found {
			t.Errorf("expected a room-packing finding, got: %v", res.Messages)
		}
	})

	t.Run("booking covering the window passes", func(t *testing.T) {
		// rooms booked 10:00–12:30 cover the whole window with 40 seats → no window finding.
		intervals := []bookedRoomInterval{
			{from: vday(10, 0), until: vday(12, 30), exahm: true, seats: 20},
			{from: vday(10, 0), until: vday(12, 30), exahm: true, seats: 20},
		}
		res := validatePreplan([]*model.PreplanExam{exam}, exahmRooms, nil, booked, 24, intervals, block)
		for _, m := range res.Messages {
			if strings.Contains(m, "bieten nur") {
				t.Errorf("did not expect a window-seats finding, got: %s", m)
			}
		}
	})
}
