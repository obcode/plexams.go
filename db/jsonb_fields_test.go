package db

import (
	"reflect"
	"strings"
	"testing"

	"github.com/obcode/plexams.go/graph/model"
)

// jsonbTypes are the model types persisted as a single jsonb column. They are
// marshalled with encoding/json, so any field encoding/json ignores is a field
// that silently does not survive a write -- unlike bson, which used its own tags
// and would happily have stored it.
var jsonbTypes = []struct {
	table string
	value any
}{
	{"generation_config", model.GenerationConfig{}},
	{"semester_config_input", model.SemesterConfigInput{}},
	{"student_prepared", model.Student{}},
	{"studentreg_upload_error", model.ZPAStudentReg{}},
	{"studentreg_upload_error", model.ZPAStudentRegError{}},
}

// knownJSONBOmissions lists fields that are deliberately not persisted, with the
// reason. Anything NOT in here that encoding/json would drop fails the test.
//
// Keep this list shrinking. An entry is a promise to resolve it, not a licence.
var knownJSONBOmissions = map[string]string{
	// LEGACY, and still the live data: 2026-WS stores 30 entries here and no
	// JointProgramAllowedTimes; loadSemesterConfig (plexams/semester_config.go:389)
	// seeds every joint program from it. The data transfer must materialise
	// JointProgramAllowedTimes from this list BEFORE the first jsonb write, after
	// which the field can be deleted from the model and this entry removed.
	"SemesterConfigInput.MucDaiAllowedTimes": "converted during the data transfer, then deleted",
}

// TestJSONBTypesLoseNoField is the structural guard for the trap that the storage
// format and the GraphQL contract are now the same tags.
//
// Under MongoDB a field could carry `json:"-"` and still be stored, because bson
// tags were separate. As a jsonb column it is simply gone. This test makes that
// class of loss a build failure instead of something to remember.
func TestJSONBTypesLoseNoField(t *testing.T) {
	for _, tc := range jsonbTypes {
		typ := reflect.TypeOf(tc.value)
		t.Run(typ.Name(), func(t *testing.T) {
			for i := range typ.NumField() {
				field := typ.Field(i)
				key := typ.Name() + "." + field.Name

				if !field.IsExported() {
					if _, known := knownJSONBOmissions[key]; !known {
						t.Errorf("%s (table %s) is unexported and would not be persisted",
							key, tc.table)
					}
					continue
				}

				name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
				if name != "-" {
					continue
				}
				if reason, known := knownJSONBOmissions[key]; known {
					t.Logf("%s is knowingly not persisted: %s", key, reason)
					continue
				}
				t.Errorf(`%s (table %s) has json:"-" and would silently not be persisted.`+
					"\nEither give it a real json tag or add it to knownJSONBOmissions with a reason.",
					key, tc.table)
			}
		})
	}
}

// TestKnownJSONBOmissionsAreStillReal keeps the allow-list honest: once a field is
// fixed or removed, its entry has to go too, otherwise the list rots into
// something nobody trusts.
func TestKnownJSONBOmissionsAreStillReal(t *testing.T) {
	present := map[string]bool{}
	for _, tc := range jsonbTypes {
		typ := reflect.TypeOf(tc.value)
		for i := range typ.NumField() {
			field := typ.Field(i)
			name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			if name == "-" || !field.IsExported() {
				present[typ.Name()+"."+field.Name] = true
			}
		}
	}

	for key := range knownJSONBOmissions {
		if !present[key] {
			t.Errorf("knownJSONBOmissions lists %s, but it is no longer omitted -- remove the entry", key)
		}
	}
}
