package plexams

import (
	"testing"

	"github.com/obcode/plexams.go/graph/model"
)

func TestAutoMucDaiLink(t *testing.T) {
	zpaByPrimuss := map[primussKey][]int{
		{"DE", 118}: {118},      // unique FK07 match
		{"GS", 131}: {517, 999}, // ambiguous (two ZPA exams)
	}
	nonZpaMap := map[primussKey]int{
		{"DE", 200}: 90001, // external exam created
	}

	cases := []struct {
		name       string
		exam       *model.MucDaiExam
		wantKind   string
		wantStatus string
		wantAncode *int
	}{
		{"fk07 unique zpa match", &model.MucDaiExam{Program: "DE", PrimussAncode: 118, PlannedBy: "FK07"}, mucDaiLinkZPA, "linked", intp(118)},
		{"fk07 ambiguous", &model.MucDaiExam{Program: "GS", PrimussAncode: 131, PlannedBy: "FK07"}, mucDaiLinkZPA, "unresolved", nil},
		{"fk07 no zpa match", &model.MucDaiExam{Program: "ID", PrimussAncode: 5, PlannedBy: "FK07"}, mucDaiLinkZPA, "unresolved", nil},
		{"external created", &model.MucDaiExam{Program: "DE", PrimussAncode: 200, PlannedBy: "FK03"}, mucDaiLinkExternal, "linked", intp(90001)},
		{"external missing", &model.MucDaiExam{Program: "DE", PrimussAncode: 201, PlannedBy: "FK08"}, mucDaiLinkExternal, "unresolved", nil},
	}
	for _, c := range cases {
		got := autoMucDaiLink(c.exam, zpaByPrimuss, nonZpaMap)
		if got.Kind != c.wantKind || got.Status != c.wantStatus {
			t.Errorf("%s: kind/status = %s/%s, want %s/%s", c.name, got.Kind, got.Status, c.wantKind, c.wantStatus)
		}
		if (got.Ancode == nil) != (c.wantAncode == nil) || (got.Ancode != nil && *got.Ancode != *c.wantAncode) {
			t.Errorf("%s: ancode = %v, want %v", c.name, got.Ancode, c.wantAncode)
		}
	}
}

func intp(i int) *int { return &i }
