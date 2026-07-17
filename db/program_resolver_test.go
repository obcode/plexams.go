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

// newResolverFor builds a programResolver directly from master programs + the set of
// programs realized (as exams_<p> collections) in a semester, bypassing Mongo.
func newResolverFor(master []*model.StudyProgram, realized ...string) *programResolver {
	r := &programResolver{
		candidatesByCode: make(map[string][]*model.StudyProgram),
		realized:         make(map[string]bool),
	}
	for _, prog := range master {
		code := prog.ZpaCode
		if code == "" {
			code = prog.Shortname
		}
		r.candidatesByCode[code] = append(r.candidatesByCode[code], prog)
	}
	for _, p := range realized {
		r.realized[p] = true
	}
	return r
}

func TestProgramResolver(t *testing.T) {
	newSemester := []*model.StudyProgram{
		sp("IF-B", "IF", "Bachelor"),
		sp("DC-B", "DC", "Bachelor"),
		sp("DC-M", "DC", "Master"),
		sp("GN", "", ""), // misc, no degree suffix → identity code
	}

	tests := []struct {
		name     string
		resolver *programResolver
		group    string
		want     string
	}{
		{
			name:     "single realized program → suffixed",
			resolver: newResolverFor(newSemester, "IF-B", "DC-B", "DC-M", "GN"),
			group:    "IF3",
			want:     "IF-B",
		},
		{
			name:     "identity code (no suffix)",
			resolver: newResolverFor(newSemester, "IF-B", "GN"),
			group:    "GN",
			want:     "GN",
		},
		{
			name:     "ambiguous dual code falls back to raw code (needs manual link)",
			resolver: newResolverFor(newSemester, "IF-B", "DC-B", "DC-M"),
			group:    "DC1",
			want:     "DC",
		},
		{
			name:     "unknown code preserved",
			resolver: newResolverFor(newSemester, "IF-B"),
			group:    "XX9",
			want:     "XX",
		},
		{
			name:     "before Primuss import: single master candidate used",
			resolver: newResolverFor(newSemester /* nothing realized */),
			group:    "IF3",
			want:     "IF-B",
		},
		{
			// old archival semester: master renamed to IF-B, but this semester's
			// collection is still exams_IF → must resolve to the raw code, not IF-B.
			name:     "old 2-letter semester stays 2-letter",
			resolver: newResolverFor(newSemester, "IF"),
			group:    "IF3",
			want:     "IF",
		},
		{
			name:     "short group returned unchanged",
			resolver: newResolverFor(newSemester, "IF-B"),
			group:    "I",
			want:     "I",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.resolver.program(tt.group); got != tt.want {
				t.Errorf("program(%q) = %q, want %q", tt.group, got, tt.want)
			}
		})
	}
}

// TestCleanupPrimussAncodesMapsStoredZpaCode guards the connect regression: ZPA
// delivers each exam's primuss ancode tagged with the raw 2-letter code (e.g. "DA"),
// which must be mapped to the internal degree-suffixed program (e.g. "DA-M") so the
// real ancode is kept instead of being dropped to -1 (which breaks connecting).
func TestCleanupPrimussAncodesMapsStoredZpaCode(t *testing.T) {
	master := []*model.StudyProgram{
		sp("DA-M", "DA", "Master"),
		sp("IF-B", "IF", "Bachelor"),
	}
	resolver := newResolverFor(master, "DA-M", "IF-B")

	exam := &model.ZPAExam{
		AnCode: 815,
		Groups: []string{"DA1"},
		// ZPA tags the primuss ancode with the raw 2-letter code:
		PrimussAncodes: []model.ZPAPrimussAncodes{{Program: "DA", Ancode: 815}},
	}
	(&DB{}).cleanupPrimussAncodes(exam, resolver)

	if len(exam.PrimussAncodes) != 1 {
		t.Fatalf("got %d primuss ancodes, want 1: %+v", len(exam.PrimussAncodes), exam.PrimussAncodes)
	}
	if got := exam.PrimussAncodes[0]; got.Program != "DA-M" || got.Ancode != 815 {
		t.Errorf("got {%q, %d}, want {DA-M, 815}: stored raw code must map to the suffixed program keeping its ancode",
			got.Program, got.Ancode)
	}
}
