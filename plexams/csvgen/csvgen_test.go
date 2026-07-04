package csvgen

import (
	"reflect"
	"testing"
	"time"

	"github.com/obcode/plexams.go/graph/model"
)

func fixedSlotTime(_, _ int) time.Time { return time.Date(2026, 1, 12, 15, 4, 0, 0, time.UTC) }

func plannedExam(module, examer string, pe *model.PlanEntry, primuss []*model.EnhancedPrimussExam, rooms []*model.PlannedRoom) *model.PlannedExam {
	return &model.PlannedExam{
		ZpaExam:      &model.ZPAExam{Module: module, MainExamer: examer},
		PlanEntry:    pe,
		PrimussExams: primuss,
		PlannedRooms: rooms,
	}
}

func section(program string, ancode, regs int) *model.EnhancedPrimussExam {
	return &model.EnhancedPrimussExam{
		Exam:        &model.PrimussExam{Program: program, AnCode: ancode},
		StudentRegs: make([]*model.EnhancedStudentReg, regs),
	}
}

func TestProgramRows(t *testing.T) {
	exams := []*model.PlannedExam{
		// ancode 502, no rooms -> placeholder row
		plannedExam("M2", "Zimmer", &model.PlanEntry{DayNumber: 1, SlotNumber: 1},
			[]*model.EnhancedPrimussExam{section("IF", 502, 3)}, nil),
		// ancode 501, two rooms (one NTA, one reserve) -> two rows, sorts before 502
		plannedExam("M1", "Albers", &model.PlanEntry{DayNumber: 1, SlotNumber: 1},
			[]*model.EnhancedPrimussExam{section("IF", 501, 2)},
			[]*model.PlannedRoom{
				{RoomName: "R1", Handicap: true, Duration: 120, StudentsInRoom: make([]string, 1)},
				{RoomName: "R2", Reserve: true, StudentsInRoom: make([]string, 3)},
			}),
		// zero-reg section in this program -> exam dropped (never enters the order)
		plannedExam("M3", "Bauer", nil, []*model.EnhancedPrimussExam{section("IF", 503, 0)}, nil),
	}
	got := ProgramRows(exams, "IF", fixedSlotTime)
	want := []CsvExam{
		{Ancode: 501, Module: "M1", MainExamer: "Albers", ExamDate: "12.01.26, 15:04 Uhr", Rooms: "R1", Comment: "NTA 120 Min., 1 Studierender eingeplant"},
		{Ancode: 501, Module: "M1", MainExamer: "Albers", ExamDate: "12.01.26, 15:04 Uhr", Rooms: "R2", Comment: "Reserveraum, nicht veröffentlichen, 3 Studierende eingeplant"},
		{Ancode: 502, Module: "M2", MainExamer: "Zimmer", ExamDate: "12.01.26, 15:04 Uhr", Rooms: "fehlen noch", Comment: ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ProgramRows =\n%#v\nwant\n%#v", got, want)
	}
}

func TestExahmRows(t *testing.T) {
	jira := "https://jira/EX-1"
	exams := []*model.PlannedExam{
		// EXaHM, one room, planned
		{
			Ancode: 111, ZpaExam: &model.ZPAExam{Module: "M1", MainExamer: "Albers"},
			MaxDuration: 90, StudentRegsCount: 20,
			Constraints:  &model.Constraints{RoomConstraints: &model.RoomConstraints{Exahm: true, KdpJiraURL: &jira}},
			PlanEntry:    &model.PlanEntry{DayNumber: 1, SlotNumber: 1},
			PlannedRooms: []*model.PlannedRoom{{RoomName: "R1.006"}},
		},
		// SEB, no rooms, not planned
		{
			Ancode: 222, ZpaExam: &model.ZPAExam{Module: "M2", MainExamer: "Zimmer"},
			MaxDuration: 60, StudentRegsCount: 5,
			Constraints: &model.Constraints{RoomConstraints: &model.RoomConstraints{Seb: true}},
		},
		// no EXaHM/SEB constraint -> skipped
		{Ancode: 333, ZpaExam: &model.ZPAExam{}, Constraints: &model.Constraints{}},
	}
	got := ExahmRows(exams, fixedSlotTime)
	want := []CsvExamEXaHM{
		{Ancode: 111, Module: "M1", MainExamer: "Albers", ExamDate: "12.01.26, 15:04 Uhr", MaxDuration: 90, Students: 20, Rooms: "[R1.006]", Type: "EXaHM", Jira: jira},
		{Ancode: 222, Module: "M2", MainExamer: "Zimmer", ExamDate: "fehlt", MaxDuration: 60, Students: 5, Rooms: "[noch nicht geplant]", Type: "SEB", Jira: "---"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExahmRows =\n%#v\nwant\n%#v", got, want)
	}
}
