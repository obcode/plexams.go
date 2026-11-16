package plexams

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
)

// Editing the form fields of a pre-exam must not drop what other mutations set
// on it: its constraints, both pair relations and the fixed slot.
func TestUpdatePreplanExamKeepsConstraintsPairsAndFixedSlot(t *testing.T) {
	pg := pgtest.NewDBWithSemester(t)
	p := &Plexams{dbClient: pg}
	ctx := context.Background()
	if err := pg.CacheTeachers([]*model.Teacher{{ID: 1, Fullname: "Prof. Dr. Oliver Braun"}}, "2026 WS"); err != nil {
		t.Fatal(err)
	}

	input := func(module, notes string) *model.PreplanExamInput {
		return &model.PreplanExamInput{
			ExamKind: "EXaHM", ExamerID: 1, Module: module,
			Programs: []string{}, ExpectedStudents: 20, Notes: &notes,
		}
	}
	a, err := p.AddPreplanExam(ctx, input("MPD", ""))
	if err != nil {
		t.Fatalf("AddPreplanExam: %v", err)
	}
	b, err := p.AddPreplanExam(ctx, input("Compilerbau", ""))
	if err != nil {
		t.Fatalf("AddPreplanExam: %v", err)
	}
	c, err := p.AddPreplanExam(ctx, input("Softwaretechnik", ""))
	if err != nil {
		t.Fatalf("AddPreplanExam: %v", err)
	}

	start := time.Date(2027, 2, 3, 10, 30, 0, 0, time.Local)
	if _, err := p.SetPreplanExamTime(ctx, a.ID, &start); err != nil {
		t.Fatalf("SetPreplanExamTime: %v", err)
	}
	if _, err := p.SetPreplanExamFixed(ctx, a.ID, true); err != nil {
		t.Fatalf("SetPreplanExamFixed: %v", err)
	}
	if _, err := p.SetPreplanExamNotSameSlot(ctx, a.ID, b.ID, true); err != nil {
		t.Fatalf("SetPreplanExamNotSameSlot: %v", err)
	}
	if _, err := p.SetPreplanExamCanShareSlot(ctx, a.ID, c.ID, true); err != nil {
		t.Fatalf("SetPreplanExamCanShareSlot: %v", err)
	}
	if _, err := p.SetPreplanExamConstraints(ctx, a.ID, &model.ConstraintsInput{
		AllowedRooms: []string{"R1.006"},
	}); err != nil {
		t.Fatalf("SetPreplanExamConstraints: %v", err)
	}

	if _, err := p.UpdatePreplanExam(ctx, a.ID, input("MPD", "nur vormittags")); err != nil {
		t.Fatalf("UpdatePreplanExam: %v", err)
	}

	got, err := pg.PreplanExam(ctx, a.ID)
	if err != nil || got == nil {
		t.Fatalf("PreplanExam: %v, %v", got, err)
	}
	if got.Notes != "nur vormittags" {
		t.Errorf("Notes = %q, the edit itself was lost", got.Notes)
	}
	if got.Constraints == nil || got.Constraints.RoomConstraints == nil || !slices.Equal(got.Constraints.RoomConstraints.AllowedRooms, []string{"R1.006"}) {
		t.Errorf("Constraints = %#v, want the allowed room back", got.Constraints)
	}
	if !got.IsFixed || got.PlannedStarttime == nil || !got.PlannedStarttime.Equal(start) {
		t.Errorf("IsFixed = %v, PlannedStarttime = %v, want fixed at %v", got.IsFixed, got.PlannedStarttime, start)
	}
	if !slices.Equal(got.NotSameSlot, []int{b.ID}) {
		t.Errorf("NotSameSlot = %v, want [%d]", got.NotSameSlot, b.ID)
	}
	if !slices.Equal(got.CanShareSlot, []int{c.ID}) {
		t.Errorf("CanShareSlot = %v, want [%d]", got.CanShareSlot, c.ID)
	}
}
