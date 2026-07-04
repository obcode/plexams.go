package email

import (
	"reflect"
	"strings"
	"testing"

	"github.com/obcode/plexams.go/graph/model"
)

func TestStudentRegsCSV(t *testing.T) {
	regs := []*model.EnhancedStudentReg{
		{Mtknr: "000123", Name: "Albers", Program: "IF", Group: "IF4A",
			ZpaStudent: &model.ZPAStudent{Gender: "w", Email: "a@hm.edu"}},
		{Mtknr: "045", Name: "Zimmer", Program: "IB", Group: "IB2B"}, // no ZpaStudent -> empty gender/email
	}
	got := string(StudentRegsCSV(regs))
	want := "Mtknr;Name;Gender;E-Mail;Studiengang;Gruppe\n" +
		"=\"000123\";Albers;w;a@hm.edu;IF;IF4A\n" +
		"=\"045\";Zimmer;;;IB;IB2B\n"
	if got != want {
		t.Errorf("StudentRegsCSV =\n%q\nwant\n%q", got, want)
	}
}

func TestStudentRegsCSVEmpty(t *testing.T) {
	got := string(StudentRegsCSV(nil))
	if got != "Mtknr;Name;Gender;E-Mail;Studiengang;Gruppe\n" {
		t.Errorf("StudentRegsCSV(nil) = %q, want just the header", got)
	}
}

func TestStudentRegsOfPrimussExams(t *testing.T) {
	primuss := []*model.EnhancedPrimussExam{
		{StudentRegs: []*model.EnhancedStudentReg{{Mtknr: "1"}, {Mtknr: "2"}}},
		{StudentRegs: nil},
		{StudentRegs: []*model.EnhancedStudentReg{{Mtknr: "3"}}},
	}
	got := StudentRegsOfPrimussExams(primuss)
	mtknrs := make([]string, len(got))
	for i, r := range got {
		mtknrs[i] = r.Mtknr
	}
	if !reflect.DeepEqual(mtknrs, []string{"1", "2", "3"}) {
		t.Errorf("flattened order = %v, want [1 2 3]", mtknrs)
	}
}

func TestRenderAssembledMarkdown(t *testing.T) {
	data := struct{ Exam *model.AssembledExam }{
		Exam: &model.AssembledExam{
			ZpaExam: &model.ZPAExam{AnCode: 123, Module: "Mathe"},
			PrimussExams: []*model.EnhancedPrimussExam{{
				Exam:        &model.PrimussExam{Program: "IF"},
				StudentRegs: []*model.EnhancedStudentReg{{Name: "Albers"}},
			}},
		},
	}
	md, err := RenderAssembledMarkdown(data)
	if err != nil {
		t.Fatalf("RenderAssembledMarkdown: %v", err)
	}
	s := string(md)
	if !strings.Contains(s, "# Anmeldungen 123. Mathe") {
		t.Errorf("markdown missing heading:\n%s", s)
	}
	if !strings.Contains(s, "1. Albers") {
		t.Errorf("markdown missing student line:\n%s", s)
	}
	if !strings.Contains(s, "Keine Nachteilsausgleiche bekannt.") {
		t.Errorf("markdown missing NTA fallback:\n%s", s)
	}
}
