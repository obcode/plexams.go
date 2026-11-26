package primuss

import (
	"reflect"
	"strings"
	"testing"

	"github.com/obcode/plexams.go/db"
)

func TestDetectPrimussFile(t *testing.T) {
	tests := []struct {
		base        string
		wantProgram string
		wantKind    string
	}{
		// the degree marker stays part of the program code (DC-B vs DC-M distinct)
		{"Prüfungsanmeldungen-IF-B-126.xlsx", "IF-B", "studentregs"},
		{"Prüfungskatalog-IF-B-126.xlsx", "IF-B", "exams"},
		{"Prüfungsplanung-IB-B-126.xlsx", "IB-B", "count"},
		{"Prüfungsüberschneidungen_nach_AnCode-IF-M-126.xlsx", "IF-M", "conflicts"},
		{"Prüfungsüberschneidungen-IF-B-126.xlsx", "IF-B", ""}, // CodeNr-keyed variant -> ignored
		{"random.xlsx", "", ""}, // no program code
		{"Prüfungsanmeldungen-DC-B-99.xlsx", "DC-B", "studentregs"},
		{"Prüfungsanmeldungen-DC-M-99.xlsx", "DC-M", "studentregs"},
	}
	for _, tt := range tests {
		t.Run(tt.base, func(t *testing.T) {
			prog, kind := detectPrimussFile(tt.base)
			if prog != tt.wantProgram || kind != tt.wantKind {
				t.Errorf("detectPrimussFile(%q) = (%q,%q), want (%q,%q)", tt.base, prog, kind, tt.wantProgram, tt.wantKind)
			}
		})
	}
}

func TestSkipReason(t *testing.T) {
	tests := []struct {
		base     string
		wantSkip bool
		contains string
	}{
		{"Prüfungsanmeldungen-IF-B-126.xlsx", false, ""},
		{"Prüfungsanmeldungen-IF-B-126.XLSX", false, ""},
		{"Pruefungsanmeldungen-IF.csv", true, "keine XLSX"},
		{"Pruefungsanmeldungen-IF-B-126.csv", true, "keine XLSX"},
		{"Pruefungsanmeldungen-IF.xlsx", true, "Studiengang"},
		{"Prüfungsanmeldungen-IF–B-126.xlsx", true, "Studiengang"}, // en dash
		{"Prüfungsüberschneidungen-IF-B-126.xlsx", true, "Listentyp"},
	}
	for _, tt := range tests {
		t.Run(tt.base, func(t *testing.T) {
			got := skipReason(tt.base)
			if (got != "") != tt.wantSkip || !strings.Contains(got, tt.contains) {
				t.Errorf("skipReason(%q) = %q, want skip=%v containing %q", tt.base, got, tt.wantSkip, tt.contains)
			}
		})
	}
}

func TestChangedAncodes(t *testing.T) {
	reg := func(ancode int, mtknr, note string) db.PrimussStudentRegRow {
		return db.PrimussStudentRegRow{
			PrimussAncode: ancode, Mtknr: mtknr,
			Raw: map[string]any{"Note": note, "gebucht": "", "nicht_zul": ""},
		}
	}

	old := []db.PrimussStudentRegRow{reg(100, "a", ""), reg(100, "b", ""), reg(200, "c", "")}

	t.Run("no change", func(t *testing.T) {
		if got := changedAncodes(old, []db.PrimussStudentRegRow{reg(100, "b", ""), reg(100, "a", ""), reg(200, "c", "")}); len(got) != 0 {
			t.Errorf("got %v, want none (order within ancode must not matter)", got)
		}
	})
	t.Run("added student flags ancode", func(t *testing.T) {
		got := changedAncodes(old, []db.PrimussStudentRegRow{reg(100, "a", ""), reg(100, "b", ""), reg(100, "x", ""), reg(200, "c", "")})
		if !reflect.DeepEqual(got, []int{100}) {
			t.Errorf("got %v, want [100]", got)
		}
	})
	t.Run("removed ancode flagged", func(t *testing.T) {
		got := changedAncodes(old, []db.PrimussStudentRegRow{reg(100, "a", ""), reg(100, "b", "")})
		if !reflect.DeepEqual(got, []int{200}) {
			t.Errorf("got %v, want [200]", got)
		}
	})
	t.Run("changed field flags ancode", func(t *testing.T) {
		got := changedAncodes(old, []db.PrimussStudentRegRow{reg(100, "a", "1.0"), reg(100, "b", ""), reg(200, "c", "")})
		if !reflect.DeepEqual(got, []int{100}) {
			t.Errorf("got %v, want [100]", got)
		}
	})
}
