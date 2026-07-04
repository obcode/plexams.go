package email

import (
	"testing"

	"github.com/obcode/plexams.go/graph/model"
)

func ewc(ancode, examerID int, examer, module string, notPlannedByMe bool) *model.ZPAExamWithConstraints {
	var c *model.Constraints
	if notPlannedByMe {
		c = &model.Constraints{NotPlannedByMe: true}
	}
	return &model.ZPAExamWithConstraints{
		ZpaExam:     &model.ZPAExam{AnCode: ancode, MainExamerID: examerID, MainExamer: examer, Module: module},
		Constraints: c,
	}
}

func TestBuildExamPlanningRecipients(t *testing.T) {
	withConstraints := []*model.ZPAExamWithConstraints{
		ewc(20, 1, "Albers", "M20", false),
		ewc(10, 1, "Albers", "M10", false),   // same examer, lower ancode -> sorts first
		ewc(30, 2, "Zimmer", "M30", true),    // NotPlannedByMe -> excluded (examer 2 gets no group)
		ewc(40, 3, "External", "M40", false), // examer not in teacher master data
	}
	teachers := []*model.Teacher{
		{ID: 1, Fullname: "Albers", FK: "FK07", Email: "a@hm.edu"},
		{ID: 2, Fullname: "Zimmer", FK: "FK07", Email: "z@hm.edu"}, // has ZPA exam, none planned -> fk07NoExams
		{ID: 5, Fullname: "NoExam", FK: "FK07", Email: "n@hm.edu"}, // FK07 but no ZPA exam -> excluded
		{ID: 6, Fullname: "OtherFK", FK: "FK09", Email: "o@hm.edu"},
	}
	allExams := []*model.ZPAExam{
		{AnCode: 10, MainExamerID: 1}, {AnCode: 20, MainExamerID: 1},
		{AnCode: 30, MainExamerID: 2}, {AnCode: 40, MainExamerID: 3},
	}

	got := BuildExamPlanningRecipients(withConstraints, teachers, allExams)

	// Expected order: withExams first (alphabetical: Albers, External), then fk07NoExams (Zimmer).
	if len(got) != 3 {
		t.Fatalf("got %d recipients, want 3: %+v", len(got), got)
	}
	if got[0].Teacher.Fullname != "Albers" || got[0].Category != "withExams" {
		t.Errorf("recipient[0] = %s/%s", got[0].Teacher.Fullname, got[0].Category)
	}
	// exams of Albers sorted by ancode
	if len(got[0].Exams) != 2 || got[0].Exams[0].Ancode != 10 || got[0].Exams[1].Ancode != 20 {
		t.Errorf("Albers exams = %+v", got[0].Exams)
	}
	if got[1].Teacher.Fullname != "External" || got[1].Category != "withExams" {
		t.Errorf("recipient[1] = %s/%s", got[1].Teacher.Fullname, got[1].Category)
	}
	// external examer got a stub teacher with the examer name
	if got[1].Teacher.ID != 3 || got[1].Teacher.Fullname != "External" {
		t.Errorf("external stub = %+v", got[1].Teacher)
	}
	if got[2].Teacher.Fullname != "Zimmer" || got[2].Category != "fk07NoExams" {
		t.Errorf("recipient[2] = %s/%s", got[2].Teacher.Fullname, got[2].Category)
	}
	if len(got[2].Exams) != 0 {
		t.Errorf("Zimmer (fk07NoExams) should have no exams, got %+v", got[2].Exams)
	}
}

func TestBuildExamPlanningRecipientsEmpty(t *testing.T) {
	got := BuildExamPlanningRecipients(nil, nil, nil)
	if len(got) != 0 {
		t.Errorf("BuildExamPlanningRecipients(nil...) = %+v, want empty", got)
	}
}
