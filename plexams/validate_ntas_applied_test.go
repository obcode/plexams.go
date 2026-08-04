package plexams

import (
	"testing"

	"github.com/obcode/plexams.go/graph/model"
)

// The case this exists for: prepare.go drops an NTA whose program disagrees with
// the registration and only writes a line to the server log. Nobody reads that,
// so the student simply does not get their Nachteilsausgleich.
func TestNtasNotAppliedReportsDroppedNta(t *testing.T) {
	nta := &model.NTA{Mtknr: "111", Name: "A", Program: "IF"}
	students := []*model.Student{
		{Mtknr: "111", Program: "IB", Nta: nil}, // prepare dropped it
	}

	got := ntasNotApplied([]*model.NTA{nta}, students)
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1", len(got))
	}
	if got[0].StudentProgram != "IB" {
		t.Errorf("StudentProgram = %q, want IB", got[0].StudentProgram)
	}
}

// An applied NTA is not a finding.
func TestNtasNotAppliedIgnoresAppliedNta(t *testing.T) {
	nta := &model.NTA{Mtknr: "111", Name: "A", Program: "IF"}
	students := []*model.Student{{Mtknr: "111", Program: "IF", Nta: nta}}

	if got := ntasNotApplied([]*model.NTA{nta}, students); len(got) != 0 {
		t.Errorf("got %v, want no findings", got)
	}
}

// A deactivated NTA is MEANT not to apply. prepare.go leaves it out of its map,
// so the student has no NTA either -- reporting that would be a false positive on
// every deactivated entry.
func TestNtasNotAppliedIgnoresDeactivated(t *testing.T) {
	nta := &model.NTA{Mtknr: "111", Name: "A", Program: "IF", Deactivated: true}
	students := []*model.Student{{Mtknr: "111", Program: "IB", Nta: nil}}

	if got := ntasNotApplied([]*model.NTA{nta}, students); len(got) != 0 {
		t.Errorf("got %v, want no findings for a deactivated NTA", got)
	}
}

// An NTA for someone not registered this semester is stale, not broken. Those are
// the old incomplete entries; reporting them would repeat in every semester's
// report until nobody reads it any more.
func TestNtasNotAppliedIgnoresUnregisteredStudent(t *testing.T) {
	nta := &model.NTA{Mtknr: "999", Name: "A", Program: ""}

	if got := ntasNotApplied([]*model.NTA{nta}, nil); len(got) != 0 {
		t.Errorf("got %v, want no findings for a student without registrations", got)
	}
}

// An empty program can never equal a registration, so a registered student with
// one always loses the NTA. This is what 6 of the 86 imported NTAs look like.
func TestNtasNotAppliedCatchesEmptyProgram(t *testing.T) {
	nta := &model.NTA{Mtknr: "111", Name: "A", Program: ""}
	students := []*model.Student{{Mtknr: "111", Program: "IF", Nta: nil}}

	got := ntasNotApplied([]*model.NTA{nta}, students)
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1", len(got))
	}
}

// A program of another faculty is legitimate and must not be reported as long as
// it matches the registration: an LR student sat an FK07 exam with an NTA in
// 2024 SS. This is the case a foreign key on nta.program would have broken.
func TestNtasNotAppliedAcceptsForeignFacultyProgram(t *testing.T) {
	nta := &model.NTA{Mtknr: "111", Name: "A", Program: "LR"}
	students := []*model.Student{{Mtknr: "111", Program: "LR", Nta: nta}}

	if got := ntasNotApplied([]*model.NTA{nta}, students); len(got) != 0 {
		t.Errorf("got %v, want no findings -- LR is a program of another faculty", got)
	}
}
