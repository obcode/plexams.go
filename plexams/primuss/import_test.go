package primuss

import (
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
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

func TestChangedAncodes(t *testing.T) {
	reg := func(ancode int, mtknr, note string) bson.M {
		return bson.M{"AnCode": ancode, "MTKNR": mtknr, "Note": note, "Stgru": "", "gebucht": "", "nicht_zul": ""}
	}

	old := []bson.M{reg(100, "a", ""), reg(100, "b", ""), reg(200, "c", "")}

	t.Run("no change", func(t *testing.T) {
		if got := changedAncodes(old, []bson.M{reg(100, "b", ""), reg(100, "a", ""), reg(200, "c", "")}); len(got) != 0 {
			t.Errorf("got %v, want none (order within ancode must not matter)", got)
		}
	})
	t.Run("added student flags ancode", func(t *testing.T) {
		got := changedAncodes(old, []bson.M{reg(100, "a", ""), reg(100, "b", ""), reg(100, "x", ""), reg(200, "c", "")})
		if !reflect.DeepEqual(got, []int{100}) {
			t.Errorf("got %v, want [100]", got)
		}
	})
	t.Run("removed ancode flagged", func(t *testing.T) {
		got := changedAncodes(old, []bson.M{reg(100, "a", ""), reg(100, "b", "")})
		if !reflect.DeepEqual(got, []int{200}) {
			t.Errorf("got %v, want [200]", got)
		}
	})
	t.Run("changed field flags ancode", func(t *testing.T) {
		got := changedAncodes(old, []bson.M{reg(100, "a", "1.0"), reg(100, "b", ""), reg(200, "c", "")})
		if !reflect.DeepEqual(got, []int{100}) {
			t.Errorf("got %v, want [100]", got)
		}
	})
}

func TestToInt(t *testing.T) {
	tests := []struct {
		in   any
		want int
	}{
		{42, 42},
		{int32(7), 7},
		{int64(9), 9},
		{float64(3.9), 3}, // truncates
		{"  15 ", 15},
		{"nope", 0},
		{nil, 0},
	}
	for _, tt := range tests {
		if got := toInt(tt.in); got != tt.want {
			t.Errorf("toInt(%v) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestTrimmedHeader(t *testing.T) {
	got := trimmedHeader([]string{" AnCode ", "Sum.", "MTKNR"}, true)
	if !reflect.DeepEqual(got, []string{"AnCode", "Sum", "MTKNR"}) {
		t.Errorf("got %v, want [AnCode Sum MTKNR]", got)
	}
	got = trimmedHeader([]string{"Sum."}, false)
	if !reflect.DeepEqual(got, []string{"Sum."}) {
		t.Errorf("without sumFix got %v, want [Sum.]", got)
	}
}
