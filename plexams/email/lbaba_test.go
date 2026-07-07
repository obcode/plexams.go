package email

import (
	"reflect"
	"testing"
	"time"

	"github.com/obcode/plexams.go/graph/model"
)

func repeaterExam(examerID int, examer, module string, notPlannedByMe bool, pe *model.PlanEntry,
	rooms []*model.PlannedRoom, primuss []*model.EnhancedPrimussExam) *model.PlannedExam {
	var c *model.Constraints
	if notPlannedByMe {
		c = &model.Constraints{NotPlannedByMe: true}
	}
	return &model.PlannedExam{
		ZpaExam:      &model.ZPAExam{IsRepeaterExam: true, MainExamerID: examerID, MainExamer: examer, Module: module},
		Constraints:  c,
		PlanEntry:    pe,
		PlannedRooms: rooms,
		PrimussExams: primuss,
	}
}

var (
	lbaSlot1 = time.Date(2026, 7, 6, 8, 0, 0, 0, time.UTC)  // 08:00
	lbaSlot2 = time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC) // 10:00
)

func lbaStart(t time.Time) *time.Time { return &t }

func TestBuildLbaRepeaterExams(t *testing.T) {
	teachers := map[int]*model.Teacher{
		1: {ID: 1, IsProf: false, Email: "lba1@hm.edu"},
		2: {ID: 2, IsProf: false, Email: "lba2@hm.edu"},
		3: {ID: 3, IsProf: true, Email: "prof@hm.edu"}, // prof -> excluded
		5: {ID: 5, IsProf: false, Email: "lba5@hm.edu"},
	}
	examer := func(id int) *model.Teacher { return teachers[id] } // id 4 -> nil (lookup fail)
	invigs := map[string]*model.Teacher{
		"R1": {ID: 10, Shortname: "Inv10", Email: "i10@hm.edu"},
		"R2": {ID: 10, Shortname: "Inv10", Email: "i10@hm.edu"}, // same ID -> deduped
		"R3": {ID: 20, Shortname: "Inv20", Email: "i20@hm.edu"},
	}
	invigilatorForRoom := func(room string, _ time.Time) *model.Teacher { return invigs[room] }

	prog := func(name string, regs int) *model.EnhancedPrimussExam {
		return &model.EnhancedPrimussExam{Exam: &model.PrimussExam{Program: name}, StudentRegs: make([]*model.EnhancedStudentReg, regs)}
	}

	exams := []*model.PlannedExam{
		// A: slot2 (later), rooms R1, R2 (dup invig), ONLINE (skipped); programs IF(2), IB(1), XX(0 -> skip)
		repeaterExam(1, "LBA One", "MA", false, &model.PlanEntry{Starttime: lbaStart(lbaSlot2)},
			[]*model.PlannedRoom{{RoomName: "R1"}, {RoomName: "R2"}, {RoomName: "ONLINE"}},
			[]*model.EnhancedPrimussExam{prog("IF", 2), prog("IB", 1), prog("XX", 0)}),
		// B: slot1 (earlier) -> sorts before A
		repeaterExam(2, "LBA Two", "MB", false, &model.PlanEntry{Starttime: lbaStart(lbaSlot1)},
			[]*model.PlannedRoom{{RoomName: "R3"}}, nil),
		// C: prof examer -> excluded
		repeaterExam(3, "Prof", "MC", false, &model.PlanEntry{Starttime: lbaStart(lbaSlot1)}, nil, nil),
		// D: not a repeater -> excluded
		{ZpaExam: &model.ZPAExam{IsRepeaterExam: false, MainExamerID: 1}},
		// E: examer lookup fails -> excluded
		repeaterExam(4, "Ghost", "ME", false, &model.PlanEntry{Starttime: lbaStart(lbaSlot1)}, nil, nil),
		// F: NotPlannedByMe -> excluded
		repeaterExam(1, "LBA One", "MF", true, &model.PlanEntry{Starttime: lbaStart(lbaSlot1)}, nil, nil),
		// G: no plan entry -> "noch nicht geplant", sorts first (zero time)
		repeaterExam(5, "LBA Five", "MG", false, nil, nil, nil),
	}

	got := BuildLbaRepeaterExams(exams, examer, invigilatorForRoom)

	// kept: G (unplanned, zero time first), B (08:00), A (10:00)
	if len(got) != 3 {
		t.Fatalf("got %d exams, want 3: %+v", len(got), got)
	}
	if got[0].Module != "MG" || got[1].Module != "MB" || got[2].Module != "MA" {
		t.Errorf("order = %s, %s, %s; want MG, MB, MA", got[0].Module, got[1].Module, got[2].Module)
	}

	// G: unplanned
	if got[0].Date != "noch nicht geplant" || got[0].Time != "" || len(got[0].Invigilators) != 0 {
		t.Errorf("G = %+v", got[0])
	}

	// A: examer email, date/time, deduped invigilators, sorted programs (count>0 only)
	a := got[2]
	if a.Examer.Email != "lba1@hm.edu" || a.Date != "Mo, 06.07.2026" || a.Time != "10:00" {
		t.Errorf("A header = %+v", a)
	}
	if len(a.Invigilators) != 1 || a.Invigilators[0].Email != "i10@hm.edu" {
		t.Errorf("A invigilators = %+v (want 1 deduped)", a.Invigilators)
	}
	wantPrograms := []LbaProgram{{Name: "IB", Count: 1}, {Name: "IF", Count: 2}}
	if !reflect.DeepEqual(a.Programs, wantPrograms) {
		t.Errorf("A programs = %+v, want %+v", a.Programs, wantPrograms)
	}
}
