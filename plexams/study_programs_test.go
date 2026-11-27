package plexams

import (
	"testing"

	"github.com/obcode/plexams.go/graph/model"
)

func TestDefaultZpaCode(t *testing.T) {
	// No viper config set → no ZPA code: the shortname is the code.
	for _, shortname := range []string{"DC", "DC-M", "IF-B", "GN", "DE"} {
		if got := defaultZpaCode(shortname); got != "" {
			t.Errorf("defaultZpaCode(%q) = %q, want empty", shortname, got)
		}
	}
}

func TestValidateStudyProgram(t *testing.T) {
	tests := []struct {
		name    string
		program model.StudyProgram
		wantErr bool
	}{
		{"fk07 two letters", model.StudyProgram{Shortname: "IF", Category: "fk07"}, false},
		{"fk07 with degree", model.StudyProgram{Shortname: "DC-M", Category: "fk07"}, false},
		{"fk07 zpa code with degree", model.StudyProgram{Shortname: "DSM", ZpaCode: "DC-M", Category: "fk07"}, false},
		{"fk07 en dash", model.StudyProgram{Shortname: "DC–M", Category: "fk07"}, true},
		{"en dash in zpa code", model.StudyProgram{Shortname: "DE", ZpaCode: "D–E", Category: "joint"}, true},
		{"fk07 lower case", model.StudyProgram{Shortname: "dc-m", Category: "fk07"}, true},
		{"fk07 bad zpa code", model.StudyProgram{Shortname: "DC", ZpaCode: "DC-X", Category: "fk07"}, true},
		{"retired fk07 free form", model.StudyProgram{Shortname: "Old1", Category: "fk07", Retired: true}, false},
		{"joint program", model.StudyProgram{Shortname: "DE", Category: "joint"}, false},
		{"space", model.StudyProgram{Shortname: "DC M", Category: "misc"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateStudyProgram(&tt.program)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateStudyProgram(%+v) = %v, wantErr %v", tt.program, err, tt.wantErr)
			}
		})
	}
}
