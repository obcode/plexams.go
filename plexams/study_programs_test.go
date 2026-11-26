package plexams

import (
	"testing"

	"github.com/obcode/plexams.go/graph/model"
)

func TestDefaultZpaCode(t *testing.T) {
	// No viper config set → only the "-B"/"-M" suffix-stripping path is exercised.
	tests := []struct {
		shortname string
		want      string
	}{
		{"DC-B", "DC"},
		{"DC-M", "DC"},
		{"IF-B", "IF"},
		{"GN", ""}, // no degree suffix → identity (empty)
		{"DE", ""}, // joint program, no suffix
		{"ABC-B", "ABC"},
	}
	for _, tt := range tests {
		t.Run(tt.shortname, func(t *testing.T) {
			if got := defaultZpaCode(tt.shortname); got != tt.want {
				t.Errorf("defaultZpaCode(%q) = %q, want %q", tt.shortname, got, tt.want)
			}
		})
	}
}

func TestValidateStudyProgram(t *testing.T) {
	tests := []struct {
		name    string
		program model.StudyProgram
		isNew   bool
		wantErr bool
	}{
		{"new fk07 suffixed", model.StudyProgram{Shortname: "DC-M", Category: "fk07"}, true, false},
		{"new fk07 with zpa code", model.StudyProgram{Shortname: "DC-M", ZpaCode: "DC", Category: "fk07"}, true, false},
		{"new fk07 zpa code with degree", model.StudyProgram{Shortname: "DC-M", ZpaCode: "DC-M", Category: "fk07"}, true, true},
		{"new fk07 without degree", model.StudyProgram{Shortname: "DC", Category: "fk07"}, true, true},
		{"new fk07 en dash", model.StudyProgram{Shortname: "DC\u2013M", Category: "fk07"}, true, true},
		{"en dash in zpa code", model.StudyProgram{Shortname: "DE", ZpaCode: "D\u2013E", Category: "joint"}, true, true},
		{"new fk07 lower case", model.StudyProgram{Shortname: "dc-m", Category: "fk07"}, true, true},
		{"existing 2-letter fk07", model.StudyProgram{Shortname: "IF", Category: "fk07"}, false, false},
		{"new retired fk07", model.StudyProgram{Shortname: "IF", Category: "fk07", Retired: true}, true, false},
		{"new joint program", model.StudyProgram{Shortname: "DE", Category: "joint"}, true, false},
		{"space", model.StudyProgram{Shortname: "DC M", Category: "misc"}, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateStudyProgram(&tt.program, tt.isNew)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateStudyProgram(%+v, %v) = %v, wantErr %v", tt.program, tt.isNew, err, tt.wantErr)
			}
		})
	}
}
