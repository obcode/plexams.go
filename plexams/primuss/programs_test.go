package primuss

import (
	"strings"
	"testing"

	"github.com/obcode/plexams.go/graph/model"
)

func prog(shortname, zpaCode, degree string) *model.StudyProgram {
	p := &model.StudyProgram{Shortname: shortname, ZpaCode: zpaCode}
	if degree != "" {
		p.Degree = &degree
	}
	return p
}

// The 2026-WS master data: two-letter shortnames, the Master DC as "DC-M".
var masterData = []*model.StudyProgram{
	prog("DA", "", "Master"),
	prog("DC", "", "Bachelor"),
	prog("DC-M", "", "Master"),
	prog("IF", "", "Bachelor"),
	prog("IG", "", "Master"),
	prog("GN", "", ""),
}

func TestResolveFileProgram(t *testing.T) {
	tests := []struct {
		code    string
		want    string
		wantErr string
	}{
		{"IF-B", "IF", ""},
		{"IG-M", "IG", ""},
		{"DA-M", "DA", ""},
		{"DC-B", "DC", ""},
		{"DC-M", "DC-M", ""}, // exact shortname wins over DC
		{"GN-B", "GN", ""},   // no degree stored: fits either
		{"IF-M", "", "es gibt nur IF (Bachelor)"},
		{"XY-B", "", "nicht angelegt"},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			got, err := resolveFileProgram(tt.code, masterData)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("resolveFileProgram(%q) = %q, %v; want error containing %q", tt.code, got, err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("resolveFileProgram(%q) = %q, %v; want %q", tt.code, got, err, tt.want)
			}
		})
	}
}

func TestResolveFileProgramWithoutMasterDC(t *testing.T) {
	// Before DC-M is created, a DC-M file must not end up in the Bachelor.
	data := []*model.StudyProgram{prog("DC", "", "Bachelor")}
	if _, err := resolveFileProgram("DC-M", data); err == nil {
		t.Fatal("DC-M resolved to the Bachelor DC")
	}
}

func TestResolveFileProgramAmbiguous(t *testing.T) {
	data := []*model.StudyProgram{prog("DC", "", ""), prog("DCX", "DC", "")}
	if _, err := resolveFileProgram("DC-B", data); err == nil || !strings.Contains(err.Error(), "mehreren") {
		t.Fatalf("want ambiguity error, got %v", err)
	}
}

func TestResolveFileProgramsCollision(t *testing.T) {
	data := []*model.StudyProgram{prog("GN", "", "")}
	if _, err := resolveFilePrograms([]string{"GN-B", "GN-M"}, data); err == nil {
		t.Fatal("two files landing on the same program must be rejected")
	}
}
