package plexams

import (
	"testing"

	"github.com/obcode/plexams.go/graph/model"
)

func genExam(ancode, regs, maxDur int, conflicts, ntas int) *model.AssembledExam {
	cs := make([]*model.ZPAConflict, conflicts)
	ns := make([]*model.NTA, ntas)
	return &model.AssembledExam{
		Ancode:           ancode,
		ZpaExam:          &model.ZPAExam{AnCode: ancode, Module: "M"},
		StudentRegsCount: regs,
		MaxDuration:      maxDur,
		Conflicts:        cs,
		Ntas:             ns,
	}
}

func TestDiffAssembledExams(t *testing.T) {
	old := []*model.AssembledExam{
		genExam(100, 42, 90, 5, 0), // unchanged
		genExam(200, 30, 90, 2, 0), // changed (regs + conflicts)
		genExam(300, 10, 90, 0, 0), // removed
	}
	newExams := []*model.AssembledExam{
		genExam(100, 42, 90, 5, 0), // unchanged
		genExam(200, 31, 90, 3, 1), // changed
		genExam(400, 5, 90, 0, 0),  // added
	}

	changes := diffAssembledExams(old, newExams)

	byAncode := make(map[int]*model.AssembledExamsChange)
	for _, c := range changes {
		byAncode[c.Ancode] = c
	}

	if _, ok := byAncode[100]; ok {
		t.Errorf("ancode 100 unchanged, should not appear in changes")
	}
	if c, ok := byAncode[400]; !ok || c.Kind != "added" {
		t.Errorf("ancode 400 should be added, got %+v", c)
	}
	if c, ok := byAncode[300]; !ok || c.Kind != "removed" {
		t.Errorf("ancode 300 should be removed, got %+v", c)
	}
	c, ok := byAncode[200]
	if !ok || c.Kind != "changed" {
		t.Fatalf("ancode 200 should be changed, got %+v", c)
	}
	// regs 30->31, conflicts 2->3, ntas 0->1 => 3 detail lines
	if len(c.Details) != 3 {
		t.Errorf("ancode 200 expected 3 details, got %v", c.Details)
	}

	// stable order by ancode
	for i := 1; i < len(changes); i++ {
		if changes[i-1].Ancode > changes[i].Ancode {
			t.Errorf("changes not sorted by ancode: %v", changes)
		}
	}
}
