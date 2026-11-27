package db

import (
	"testing"

	"github.com/obcode/plexams.go/graph/model"
)

func sp(shortname, zpaCode, degree string) *model.StudyProgram {
	p := &model.StudyProgram{Shortname: shortname, ZpaCode: zpaCode}
	if degree != "" {
		d := degree
		p.Degree = &d
	}
	return p
}

// master mirrors the 2026-WS master data: two-letter shortnames, plus the
// Master DC under the code ZPA uses for it.
var master = []*model.StudyProgram{
	sp("DA", "", "Master"),
	sp("DC", "", "Bachelor"),
	sp("DC-M", "", "Master"),
	sp("IF", "", "Bachelor"),
	sp("IG", "", "Master"),
	sp("GN", "", "Bachelor"),
}

func TestProgramResolver(t *testing.T) {
	r := newProgramResolverFrom(master)
	tests := []struct {
		group string
		want  string
	}{
		{"IF3", "IF"},
		{"IF1A", "IF"},
		{"DC", "DC"},
		{"DC5", "DC"},
		{"DC-M", "DC-M"}, // the longest matching code wins
		{"DC-M1", "DC-M"},
		{"DA1", "DA"},
		{"GN", "GN"},
		{"XX9", "XX"}, // unknown: first two letters, as before
		{"I", "I"},
	}
	for _, tt := range tests {
		t.Run(tt.group, func(t *testing.T) {
			if got := r.program(tt.group); got != tt.want {
				t.Errorf("program(%q) = %q, want %q", tt.group, got, tt.want)
			}
		})
	}
}

func TestProgramResolverZpaCode(t *testing.T) {
	// A program may carry a ZPA code different from its shortname.
	r := newProgramResolverFrom([]*model.StudyProgram{sp("DSM", "DC-M", "Master"), sp("DC", "", "Bachelor")})
	if got := r.program("DC-M1"); got != "DSM" {
		t.Errorf("program(DC-M1) = %q, want DSM", got)
	}
	if got := r.program("DC5"); got != "DC" {
		t.Errorf("program(DC5) = %q, want DC", got)
	}
}

func TestProgramResolverAmbiguousCode(t *testing.T) {
	r := newProgramResolverFrom([]*model.StudyProgram{sp("X1", "DC", ""), sp("X2", "DC", "")})
	if got := r.program("DC5"); got != "DC" {
		t.Errorf("ambiguous code must stay raw, got %q", got)
	}
}

func cleanup(groups []string, ancodes ...model.ZPAPrimussAncodes) []model.ZPAPrimussAncodes {
	exam := &model.ZPAExam{AnCode: 815, Groups: groups, PrimussAncodes: ancodes}
	cleanupPrimussAncodes(exam, newProgramResolverFrom(master))
	return exam.PrimussAncodes
}

func TestCleanupPrimussAncodes(t *testing.T) {
	got := cleanup([]string{"DA", "DC-M", "IG"},
		model.ZPAPrimussAncodes{Program: "DA", Ancode: 11},
		model.ZPAPrimussAncodes{Program: "DC-M", Ancode: 12},
		model.ZPAPrimussAncodes{Program: "IG", Ancode: 13})
	want := []model.ZPAPrimussAncodes{{Program: "DA", Ancode: 11}, {Program: "DC-M", Ancode: 12}, {Program: "IG", Ancode: 13}}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %+v, want %+v", got, want)
		}
	}
}

// ZPA might tag the ancode of a Master exam with the plain two letters.
func TestCleanupPrimussAncodesPlainTagForSuffixedProgram(t *testing.T) {
	got := cleanup([]string{"DA1", "DC-M1"}, model.ZPAPrimussAncodes{Program: "DC", Ancode: 12})
	if len(got) != 2 || got[1] != (model.ZPAPrimussAncodes{Program: "DC-M", Ancode: 12}) {
		t.Errorf("got %+v, want DC-M with ancode 12", got)
	}
}

// When the exam has the Bachelor DC itself, a "DC" tag belongs to it.
func TestCleanupPrimussAncodesPlainTagPrefersExactProgram(t *testing.T) {
	got := cleanup([]string{"DC5", "DC-M1"}, model.ZPAPrimussAncodes{Program: "DC", Ancode: 12})
	if len(got) != 2 || got[0] != (model.ZPAPrimussAncodes{Program: "DC", Ancode: 12}) || got[1].Ancode != -1 {
		t.Errorf("got %+v, want DC:12 and DC-M:-1", got)
	}
}
