package email

import (
	"reflect"
	"testing"
	"time"

	"github.com/obcode/plexams.go/graph/model"
)

func ptr(s string) *string { return &s }

func kdpExam(ancode int, exahm bool, day, slot int, rooms []*model.PlannedRoom) *model.PlannedExam {
	return &model.PlannedExam{
		Ancode:       ancode,
		ZpaExam:      &model.ZPAExam{Module: "M" + string(rune('0'+ancode%10)), MainExamer: "Prof"},
		Constraints:  &model.Constraints{RoomConstraints: &model.RoomConstraints{Exahm: exahm, Seb: !exahm}},
		PlanEntry:    &model.PlanEntry{DayNumber: day, SlotNumber: slot},
		PlannedRooms: rooms,
	}
}

func students(n int) []string { return make([]string, n) }

// slotTimes gives slot (1,1) an earlier start than (1,2), so ordering is testable.
func testSlotTime(day, slot int) time.Time {
	return time.Date(2026, 7, 6, 8+slot, 0, 0, 0, time.UTC)
}

func TestBuildKdpAggregation(t *testing.T) {
	exams := []*model.PlannedExam{
		// EXaHM in T3.023, slot (1,1): 2 normal seats (90 min) + 1 reserve seat
		kdpExam(111, true, 1, 1, []*model.PlannedRoom{
			{RoomName: "T3.023", Duration: 90, StudentsInRoom: students(2)},
			{RoomName: "T3.023", Duration: 90, Reserve: true, StudentsInRoom: students(1)},
		}),
		// SEB in T3.023, same slot: 1 NTA seat (120 min)
		kdpExam(222, false, 1, 1, []*model.PlannedRoom{
			{RoomName: "T3.023", Duration: 120, NtaMtknr: ptr("m1"), StudentsInRoom: students(1)},
		}),
		// EXaHM in a later slot, ONLINE room is ignored -> contributes no slot
		kdpExam(333, true, 1, 2, []*model.PlannedRoom{
			{RoomName: "ONLINE", Duration: 90, StudentsInRoom: students(4)},
		}),
		// not EXaHM/SEB -> skipped entirely
		{Ancode: 444, ZpaExam: &model.ZPAExam{}, PlanEntry: &model.PlanEntry{DayNumber: 1, SlotNumber: 1}},
		// EXaHM without a plan entry -> skipped
		{Ancode: 555, ZpaExam: &model.ZPAExam{}, Constraints: &model.Constraints{RoomConstraints: &model.RoomConstraints{Exahm: true}}},
	}

	slots, csv := BuildKdp(exams, testSlotTime)

	// only one slot has a (non-ONLINE) room
	if len(slots) != 1 {
		t.Fatalf("got %d slots, want 1", len(slots))
	}
	s := slots[0]
	if s.Date != "Mo, 06.07.2026" || s.Time != "09:00" {
		t.Errorf("slot date/time = %q %q", s.Date, s.Time)
	}
	if len(s.Rooms) != 1 || s.Rooms[0].RoomName != "T3.023" {
		t.Fatalf("rooms = %+v", s.Rooms)
	}
	examsInRoom := s.Rooms[0].Exams
	if len(examsInRoom) != 2 {
		t.Fatalf("got %d exams in room, want 2", len(examsInRoom))
	}
	// sorted by ancode: 111 then 222
	if examsInRoom[0].Ancode != 111 || examsInRoom[1].Ancode != 222 {
		t.Errorf("ancode order = %d, %d", examsInRoom[0].Ancode, examsInRoom[1].Ancode)
	}

	a := examsInRoom[0]
	if a.Seats != 2 || a.NtaSeats != 0 || a.ReserveSeats != 1 {
		t.Errorf("exam 111 seats=%d nta=%d reserve=%d", a.Seats, a.NtaSeats, a.ReserveSeats)
	}
	if a.Detail != "2× 90 min, Reserve 1" {
		t.Errorf("exam 111 detail = %q", a.Detail)
	}

	b := examsInRoom[1]
	if b.Seats != 1 || b.NtaSeats != 1 || b.ReserveSeats != 0 {
		t.Errorf("exam 222 seats=%d nta=%d reserve=%d", b.Seats, b.NtaSeats, b.ReserveSeats)
	}
	if b.Detail != "1× NTA (120 min)" {
		t.Errorf("exam 222 detail = %q", b.Detail)
	}

	// CSV: one row per (room, exam)
	want := []CsvKdpRoom{
		{Tag: "Mo", Datum: "06.07.2026", Beginn: "09:00", Raum: "T3.023", Ancode: 111, Modul: a.Module,
			Erstpruefer: "Prof", Typ: "EXaHM", Plaetze: 2, NTAPlaetze: 0, Reserve: 1, DauerMin: 90, NTADauern: ""},
		{Tag: "Mo", Datum: "06.07.2026", Beginn: "09:00", Raum: "T3.023", Ancode: 222, Modul: b.Module,
			Erstpruefer: "Prof", Typ: "SEB", Plaetze: 1, NTAPlaetze: 1, Reserve: 0, DauerMin: 0, NTADauern: "120"},
	}
	if !reflect.DeepEqual(csv, want) {
		t.Errorf("csv =\n%+v\nwant\n%+v", csv, want)
	}
}

func TestBuildKdpEmpty(t *testing.T) {
	slots, csv := BuildKdp(nil, testSlotTime)
	if len(slots) != 0 || len(csv) != 0 {
		t.Errorf("BuildKdp(nil) = %d slots, %d csv; want 0, 0", len(slots), len(csv))
	}
}

func TestBuildKdpSlotOrdering(t *testing.T) {
	// exam in the later slot listed first in the input; output must be time-ordered.
	exams := []*model.PlannedExam{
		kdpExam(200, true, 1, 2, []*model.PlannedRoom{{RoomName: "R2", Duration: 90, StudentsInRoom: students(1)}}),
		kdpExam(100, true, 1, 1, []*model.PlannedRoom{{RoomName: "R1", Duration: 90, StudentsInRoom: students(1)}}),
	}
	slots, _ := BuildKdp(exams, testSlotTime)
	if len(slots) != 2 || slots[0].Time != "09:00" || slots[1].Time != "10:00" {
		t.Errorf("slot ordering = %+v", []string{slots[0].Time, slots[1].Time})
	}
}
