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
