package plexams

import (
	"testing"

	"github.com/obcode/plexams.go/graph/model"
)

func TestAutoJointLink(t *testing.T) {
	zpaByPrimuss := map[primussKey][]int{
		{"DE", 118}: {118},      // unique FK07 match
		{"GS", 131}: {517, 999}, // ambiguous (two ZPA exams)
	}
	externalMap := map[primussKey]int{
		{"DE", 200}: 90001, // external exam created
	}

	cases := []struct {
		name       string
		exam       *model.JointExam
		wantKind   string
		wantStatus string
		wantAncode *int
	}{
		{"fk07 unique zpa match", &model.JointExam{Program: "DE", PrimussAncode: 118, PlannedBy: "FK07"}, jointLinkZPA, "linked", intp(118)},
		{"fk07 ambiguous", &model.JointExam{Program: "GS", PrimussAncode: 131, PlannedBy: "FK07"}, jointLinkZPA, "unresolved", nil},
		{"fk07 no zpa match", &model.JointExam{Program: "ID", PrimussAncode: 5, PlannedBy: "FK07"}, jointLinkZPA, "unresolved", nil},
		{"external created", &model.JointExam{Program: "DE", PrimussAncode: 200, PlannedBy: "FK03"}, jointLinkExternal, "linked", intp(90001)},
		{"external missing", &model.JointExam{Program: "DE", PrimussAncode: 201, PlannedBy: "FK08"}, jointLinkExternal, "unresolved", nil},
	}
	for _, c := range cases {
		got := autoJointLink(c.exam, zpaByPrimuss, externalMap)
		if got.Kind != c.wantKind || got.Status != c.wantStatus {
			t.Errorf("%s: kind/status = %s/%s, want %s/%s", c.name, got.Kind, got.Status, c.wantKind, c.wantStatus)
		}
		if (got.Ancode == nil) != (c.wantAncode == nil) || (got.Ancode != nil && *got.Ancode != *c.wantAncode) {
			t.Errorf("%s: ancode = %v, want %v", c.name, got.Ancode, c.wantAncode)
		}
	}
}

func intp(i int) *int { return &i }
