package pdfgen

import (
	"reflect"
	"testing"
	"time"

	"github.com/obcode/plexams.go/graph/model"
)

func zpaExam(ancode int, module, examer string, groups []string, typeFull string) *model.ZPAExam {
	return &model.ZPAExam{AnCode: ancode, Module: module, MainExamer: examer, Groups: groups, ExamTypeFull: typeFull}
}

func TestExamsToPlanRows(t *testing.T) {
	exams := []*model.ZPAExam{
		zpaExam(2, "M2", "Zimmer", []string{"IB4"}, ""),
		zpaExam(1, "M1", "Albers", []string{"IF2"}, ""),
		zpaExam(3, "M3", "Albers", []string{"IF4"}, ""),
	}
	got := ExamsToPlanRows(exams)
	// grouped+sorted by examer name: Albers (1, 3), then Zimmer (2)
	want := [][]string{
		{"1", "M1", "Albers", "[IF2]"},
		{"3", "M3", "Albers", "[IF4]"},
		{"2", "M2", "Zimmer", "[IB4]"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExamsToPlanRows =\n%v\nwant\n%v", got, want)
	}
}

func TestSameModulNamesRows(t *testing.T) {
	exams := []*model.ZPAExam{
		zpaExam(10, "Mathe", "Albers", []string{"IF2"}, "schriftlich"),
		zpaExam(11, "Mathe", "Zimmer", []string{"IB2"}, "schriftlich"),
		zpaExam(20, "Physik", "Braun", []string{"IF4"}, "praktisch"),
	}
	got := SameModulNamesRows(exams)
	want := [][]string{
		{"Mathe", "10", "Albers", "[IF2]", "schriftlich"},
		{"", "11", "Zimmer", "[IB2]", "schriftlich"}, // continuation row, module blank
		{"", "", "", "", ""},                         // separator
		{"Physik", "20", "Braun", "[IF4]", "praktisch"},
		{"", "", "", "", ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SameModulNamesRows =\n%v\nwant\n%v", got, want)
	}
}

func TestSameSlotAncodes(t *testing.T) {
	exams := []*model.ZPAExamWithConstraints{
		{ZpaExam: zpaExam(1, "M1", "A", nil, ""), Constraints: &model.Constraints{SameSlot: []int{5, 3}}},
		{ZpaExam: zpaExam(2, "M2", "B", nil, ""), Constraints: &model.Constraints{SameSlot: []int{3}}}, // 3 deduped
		{ZpaExam: zpaExam(4, "M4", "C", nil, ""), Constraints: nil},                                    // no constraints
	}
	got := SameSlotAncodes(exams)
	if !reflect.DeepEqual(got, []int{3, 5}) {
		t.Errorf("SameSlotAncodes = %v, want [3 5]", got)
	}
}

func TestConstraintsRows(t *testing.T) {
	day := time.Date(2026, 1, 15, 0, 0, 0, 0, time.Local)
	exams := []*model.ZPAExamWithConstraints{
		{ // dropped: NotPlannedByMe
			ZpaExam:     zpaExam(9, "Skip", "Albers", nil, ""),
			Constraints: &model.Constraints{NotPlannedByMe: true},
		},
		{
			ZpaExam: zpaExam(1, "Mathe", "Albers", []string{"IF2"}, ""),
			Constraints: &model.Constraints{
				Online:          true,
				ExcludeDays:     []*time.Time{&day},
				SameSlot:        []int{7},
				RoomConstraints: &model.RoomConstraints{Seb: true, Exahm: true},
			},
		},
	}
	sameSlot := map[int]*model.ZPAExam{7: zpaExam(7, "Parallel", "Braun", []string{"IB2"}, "")}

	got := ConstraintsRows(exams, sameSlot)
	want := [][]string{
		{"", "", "", ""}, // leading blank row
		{"1", "Albers", "Mathe", "[IF2]", "Constraints:"},
		{"", "", "", "", "- Fernprüfung gem. BayFEV "},
		{"", "", "", "", "- Nicht am 15.01.26"},
		{"", "", "", "", "- zeitgleich: 7. Braun, Parallel, [IB2]"},
		{"", "", "", "", "- SafeExamBrowser"},
		{"", "", "", "", "- EXaHM"},
		{"", "", "", ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ConstraintsRows =\n%#v\nwant\n%#v", got, want)
	}
}
