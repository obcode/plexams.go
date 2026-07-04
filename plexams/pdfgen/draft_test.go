package pdfgen

import (
	"reflect"
	"testing"
	"time"

	"github.com/obcode/plexams.go/graph/model"
)

func TestFormatTermin(t *testing.T) {
	// 2026-01-12 is a Monday.
	got := FormatTermin(time.Date(2026, 1, 12, 15, 4, 0, 0, time.UTC))
	if got != "Mo. 12.01.26, 15:04 Uhr" {
		t.Errorf("FormatTermin = %q, want %q", got, "Mo. 12.01.26, 15:04 Uhr")
	}
}

func plannedExam(ancode int, module, examer string, pe *model.PlanEntry, primuss []*model.EnhancedPrimussExam) *model.PlannedExam {
	return &model.PlannedExam{
		Ancode:       ancode,
		ZpaExam:      &model.ZPAExam{Module: module, MainExamer: examer},
		PlanEntry:    pe,
		PrimussExams: primuss,
	}
}

func section(program string, ancode int, regs int) *model.EnhancedPrimussExam {
	studentRegs := make([]*model.EnhancedStudentReg, regs)
	return &model.EnhancedPrimussExam{
		Exam:        &model.PrimussExam{Program: program, AnCode: ancode},
		StudentRegs: studentRegs,
	}
}

// fixedSlotTime returns a Monday 15:04 for any (day, slot), enough to check formatting.
func fixedSlotTime(_, _ int) time.Time {
	return time.Date(2026, 1, 12, 15, 4, 0, 0, time.UTC)
}

func TestProgramRows(t *testing.T) {
	exams := []*model.PlannedExam{
		// no plan entry -> "fehlt noch"; program section overrides the ancode to 501
		plannedExam(3, "M3", "Zimmer", nil, []*model.EnhancedPrimussExam{section("IF", 501, 2)}),
		// planned -> formatted Termin; sorted before ancode 501
		plannedExam(1, "M1", "Albers", &model.PlanEntry{DayNumber: 1, SlotNumber: 1}, nil),
	}
	got := ProgramRows(exams, "IF", fixedSlotTime)
	want := [][]string{
		{"1", "M1", "Albers", "Mo. 12.01.26, 15:04 Uhr"},
		{"501", "M3", "Zimmer", "fehlt noch"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ProgramRows =\n%v\nwant\n%v", got, want)
	}
}

func TestExamRows(t *testing.T) {
	exams := []*model.PlannedExam{
		plannedExam(3, "M3", "Zimmer", nil, nil),
		plannedExam(1, "M1", "Albers", &model.PlanEntry{DayNumber: 1, SlotNumber: 1}, nil),
	}
	got := ExamRows(exams, fixedSlotTime)
	want := [][]string{
		{"1", "M1", "Albers", "Mo. 12.01.26, 15:04 Uhr"},
		{"3", "M3", "Zimmer", "fehlt noch"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExamRows =\n%v\nwant\n%v", got, want)
	}
}

func exahmExam(ancode int, module, examer string, pe *model.PlanEntry, regs int,
	exahm bool, plannedRooms []*model.PlannedRoom) *model.PlannedExam {
	return &model.PlannedExam{
		Ancode:           ancode,
		ZpaExam:          &model.ZPAExam{Module: module, MainExamer: examer},
		PlanEntry:        pe,
		StudentRegsCount: regs,
		Constraints:      &model.Constraints{RoomConstraints: &model.RoomConstraints{Exahm: exahm, Seb: !exahm}},
		PlannedRooms:     plannedRooms,
	}
}

func TestExahmRowsByAncode(t *testing.T) {
	exams := []*model.PlannedExam{
		exahmExam(20, "M20", "Zimmer", nil, 7, false, nil),
		exahmExam(10, "M10", "Albers", &model.PlanEntry{DayNumber: 2, SlotNumber: 3}, 12, true,
			[]*model.PlannedRoom{{RoomName: "R1.001"}, {RoomName: "R1.002"}}),
	}
	prePlanned := map[int][]string{20: {"K1.001"}}
	// input order kept (no sort by date)
	got := ExahmRows(exams, false, fixedSlotTime, prePlanned)
	want := [][]string{
		{"20", "M20", "Zimmer", "fehlt noch", "SEB", "7", "K1.001"},
		{"10", "M10", "Albers", "Mo. 12.01.26, 15:04 Uhr", "EXaHM", "12", "R1.001, R1.002"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExahmRows =\n%v\nwant\n%v", got, want)
	}
}

func TestExahmRowsByDate(t *testing.T) {
	// unplanned exams sort last; planned ordered by (day, slot, ancode).
	exams := []*model.PlannedExam{
		exahmExam(30, "M30", "Zimmer", nil, 1, false, nil),
		exahmExam(20, "M20", "Albers", &model.PlanEntry{DayNumber: 2, SlotNumber: 1}, 2, true, nil),
		exahmExam(10, "M10", "Bauer", &model.PlanEntry{DayNumber: 1, SlotNumber: 5}, 3, false, nil),
	}
	got := ExahmRows(exams, true, fixedSlotTime, nil)
	gotAncodes := []string{got[0][0], got[1][0], got[2][0]}
	want := []string{"10", "20", "30"}
	if !reflect.DeepEqual(gotAncodes, want) {
		t.Errorf("ExahmRows by date order = %v, want %v", gotAncodes, want)
	}
	// rooms with neither planned nor pre-planned -> "fehlen noch"
	if got[0][6] != "fehlen noch" {
		t.Errorf("rooms = %q, want %q", got[0][6], "fehlen noch")
	}
}

func TestExahmHeading(t *testing.T) {
	if got := ExahmHeading(false); got != "Prüfungen mit EXaHM/SEB, sortiert nach AnCode" {
		t.Errorf("ExahmHeading(false) = %q", got)
	}
	if got := ExahmHeading(true); got != "Prüfungen mit EXaHM/SEB, sortiert nach Datum" {
		t.Errorf("ExahmHeading(true) = %q", got)
	}
}

func TestExahmRowsDoesNotMutateInput(t *testing.T) {
	exams := []*model.PlannedExam{
		exahmExam(20, "M20", "Albers", &model.PlanEntry{DayNumber: 2, SlotNumber: 1}, 1, true, nil),
		exahmExam(10, "M10", "Bauer", &model.PlanEntry{DayNumber: 1, SlotNumber: 1}, 1, false, nil),
	}
	_ = ExahmRows(exams, true, fixedSlotTime, nil)
	if exams[0].Ancode != 20 || exams[1].Ancode != 10 {
		t.Errorf("ExahmRows mutated the input slice order: %d, %d", exams[0].Ancode, exams[1].Ancode)
	}
}

func TestProgramRowsSectionWithoutRegsStopsListing(t *testing.T) {
	// matching-program section with zero registrations breaks the whole listing
	// (preserving the original behaviour) — the exam after it is dropped.
	exams := []*model.PlannedExam{
		plannedExam(1, "M1", "Albers", nil, []*model.EnhancedPrimussExam{section("IF", 1, 0)}),
		plannedExam(2, "M2", "Zimmer", nil, nil),
	}
	got := ProgramRows(exams, "IF", fixedSlotTime)
	if len(got) != 0 {
		t.Errorf("ProgramRows = %v, want empty (listing stopped at zero-reg section)", got)
	}
}
