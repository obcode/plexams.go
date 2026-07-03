package preplancalc

import (
	"reflect"
	"testing"

	"github.com/obcode/plexams.go/graph/model"
)

func TestNormRoomName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"R1.006", "R1.006"},
		{" r1.006 ", "R1.006"},
		{"T 3.014", "T3.014"},
		{"  a b  c ", "ABC"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := NormRoomName(tt.in); got != tt.want {
			t.Errorf("NormRoomName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestTotalSeats(t *testing.T) {
	rooms := []RoomCapacity{{Name: "A", Seats: 30}, {Name: "B", Seats: 20}}
	if got := TotalSeats(rooms); got != 50 {
		t.Errorf("TotalSeats = %d, want 50", got)
	}
	if got := TotalSeats(nil); got != 0 {
		t.Errorf("TotalSeats(nil) = %d, want 0", got)
	}
}

func TestRoomsToBook(t *testing.T) {
	rooms := []RoomCapacity{{Name: "T3.014", Seats: 30}, {Name: "T3.015", Seats: 20}, {Name: "T3.016", Seats: 10}}

	t.Run("picks largest first until gap covered", func(t *testing.T) {
		got := RoomsToBook(rooms, 35, nil)
		if !reflect.DeepEqual(got, []string{"T3.014", "T3.015"}) {
			t.Errorf("got %v, want [T3.014 T3.015]", got)
		}
	})
	t.Run("skips already-booked rooms", func(t *testing.T) {
		got := RoomsToBook(rooms, 35, map[string]bool{"T3.014": true})
		if !reflect.DeepEqual(got, []string{"T3.015", "T3.016"}) {
			t.Errorf("got %v, want [T3.015 T3.016]", got)
		}
	})
	t.Run("nothing to book when gap <= 0", func(t *testing.T) {
		if got := RoomsToBook(rooms, 0, nil); len(got) != 0 {
			t.Errorf("got %v, want empty", got)
		}
	})
}

func preExam(kind string, students int, allowedRooms ...string) *model.PreplanExam {
	pe := &model.PreplanExam{ExamKind: kind, ExpectedStudents: students}
	if len(allowedRooms) > 0 {
		pe.Constraints = &model.Constraints{RoomConstraints: &model.RoomConstraints{AllowedRooms: allowedRooms}}
	}
	return pe
}

func TestRoomsForKind(t *testing.T) {
	rooms := []RoomCapacity{{Name: "T3.014", Seats: 30}, {Name: "T3.015", Seats: 20}}

	t.Run("no restriction keeps full pool", func(t *testing.T) {
		got := RoomsForKind([]*model.PreplanExam{preExam("EXaHM", 10)}, "EXaHM", rooms)
		if len(got) != 2 {
			t.Errorf("got %d rooms, want 2", len(got))
		}
	})
	t.Run("an unrestricted exam keeps the full pool even if another restricts", func(t *testing.T) {
		exams := []*model.PreplanExam{preExam("EXaHM", 10, "T3.014"), preExam("EXaHM", 5)}
		if got := RoomsForKind(exams, "EXaHM", rooms); len(got) != 2 {
			t.Errorf("got %d rooms, want 2 (one exam unrestricted)", len(got))
		}
	})
	t.Run("all restricted narrows to the union", func(t *testing.T) {
		exams := []*model.PreplanExam{preExam("EXaHM", 10, "T3.014")}
		got := RoomsForKind(exams, "EXaHM", rooms)
		if len(got) != 1 || got[0].Name != "T3.014" {
			t.Errorf("got %v, want [T3.014]", got)
		}
	})
	t.Run("only exams of the kind matter", func(t *testing.T) {
		exams := []*model.PreplanExam{preExam("SEB", 10, "T3.014")}
		if got := RoomsForKind(exams, "EXaHM", rooms); len(got) != 2 {
			t.Errorf("got %d rooms, want 2 (restriction is on a SEB exam)", len(got))
		}
	})
}

func TestKindNeed(t *testing.T) {
	rooms := []RoomCapacity{{Name: "T3.014", Seats: 30}, {Name: "T3.015", Seats: 20}}
	exams := []*model.PreplanExam{preExam("EXaHM", 25), preExam("EXaHM", 10), preExam("SEB", 5)}

	need := KindNeed(exams, "EXaHM", rooms)
	if need.ExamCount != 2 {
		t.Errorf("ExamCount = %d, want 2", need.ExamCount)
	}
	if need.SeatsNeeded != 35 {
		t.Errorf("SeatsNeeded = %d, want 35", need.SeatsNeeded)
	}
	if need.SeatsAvailable != 50 {
		t.Errorf("SeatsAvailable = %d, want 50", need.SeatsAvailable)
	}
	// 35 seats needed -> T3.014 (30) then T3.015 (20) suggested
	if !reflect.DeepEqual(need.Rooms, []string{"T3.014", "T3.015"}) {
		t.Errorf("Rooms = %v, want [T3.014 T3.015]", need.Rooms)
	}
}

func TestApplyBooking(t *testing.T) {
	rooms := []RoomCapacity{{Name: "T3.014", Seats: 30}, {Name: "T3.015", Seats: 20}}
	need := &model.PreplanKindNeed{SeatsNeeded: 35, RoomsToBook: []string{}}

	ApplyBooking(need, 30, rooms, map[string]bool{"T3.014": true})
	if need.SeatsBooked != 30 {
		t.Errorf("SeatsBooked = %d, want 30", need.SeatsBooked)
	}
	// gap 5, T3.014 already booked -> suggest T3.015
	if !reflect.DeepEqual(need.RoomsToBook, []string{"T3.015"}) {
		t.Errorf("RoomsToBook = %v, want [T3.015]", need.RoomsToBook)
	}

	ApplyBooking(need, 40, rooms, nil) // over-booked -> gap clamped to 0
	if len(need.RoomsToBook) != 0 {
		t.Errorf("RoomsToBook = %v, want empty when fully booked", need.RoomsToBook)
	}
}

func TestProgramConflicts(t *testing.T) {
	ex := func(id int, module string, programs ...string) *model.PreplanExam {
		return &model.PreplanExam{ID: id, Module: module, Programs: programs}
	}
	exams := []*model.PreplanExam{
		ex(1, "Math", "IF", "IB"),
		ex(2, "Phys", "IB"), // IB shared with exam 1
		ex(3, "Chem", "DC"), // no clash
	}
	got := ProgramConflicts(exams)
	if len(got) != 1 {
		t.Fatalf("got %d conflicts, want 1", len(got))
	}
	if got[0].Program != "IB" || !reflect.DeepEqual(got[0].PreplanExamIDs, []int{1, 2}) {
		t.Errorf("conflict = %+v, want program IB, ids [1 2]", got[0])
	}
}
