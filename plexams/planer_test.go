package plexams

import (
	"testing"

	"github.com/obcode/plexams.go/db"
)

// The configured default is the planner of every semester that has no override --
// that is the whole two-tier arrangement in one test.
func TestMergePlanerWithoutOverride(t *testing.T) {
	def := Planer{Name: "Prüfungsplanung FK07", Email: "oliver.braun@hm.edu"}

	got := mergePlaner(def, nil)

	if got.Name != def.Name || got.Email != def.Email {
		t.Errorf("identity = %q/%q, want the configured %q/%q", got.Name, got.Email, def.Name, def.Email)
	}
	if !got.Inherited {
		t.Error("Inherited = false, want true -- there is no override")
	}
}

func TestMergePlanerWithOwnIdentity(t *testing.T) {
	def := Planer{Name: "Prüfungsplanung FK07", Email: "oliver.braun@hm.edu"}

	got := mergePlaner(def, &db.SemesterPlaner{
		Name:  strptr("Vertretung FK07"),
		Email: strptr("vertretung@hm.edu"),
	})

	if got.Name != "Vertretung FK07" || got.Email != "vertretung@hm.edu" {
		t.Errorf("identity = %q/%q, want the semester's own", got.Name, got.Email)
	}
	if got.Inherited {
		t.Error("Inherited = true, want false -- the semester has its own identity")
	}
}

// A semester may keep the configured planner and still redirect its dry runs: the
// four sender overrides are independent of the identity and of each other.
func TestMergePlanerSenderOverridesAreIndependentOfTheIdentity(t *testing.T) {
	def := Planer{Name: "Prüfungsplanung FK07", Email: "oliver.braun@hm.edu"}

	got := mergePlaner(def, &db.SemesterPlaner{
		TestMail: strptr("probelauf@hm.edu"),
		Cc:       strptr("sekretariat@hm.edu"),
	})

	if got.Name != def.Name || got.Email != def.Email {
		t.Errorf("identity = %q/%q, want the configured one to survive", got.Name, got.Email)
	}
	if !got.Inherited {
		t.Error("Inherited = false, want true -- only the sender overrides were set")
	}
	if got.TestMail != "probelauf@hm.edu" {
		t.Errorf("TestMail = %q, want %q", got.TestMail, "probelauf@hm.edu")
	}
	if got.Cc != "sekretariat@hm.edu" {
		t.Errorf("Cc = %q, want %q", got.Cc, "sekretariat@hm.edu")
	}
	// Unset ones must stay empty so the Sender falls through to smtp.* and then to
	// its derived default -- an empty string is "unset", not "send nowhere".
	if got.NoreplyMail != "" || got.NoreplyName != "" {
		t.Errorf("noreply = %q/%q, want empty (unset)", got.NoreplyMail, got.NoreplyName)
	}
}

// Half an identity cannot be stored (the table constraint refuses it), but if it
// ever reached here it must not produce "Prüfungsplanung FK07 <vertretung@hm.edu>".
func TestMergePlanerIgnoresHalfAnIdentity(t *testing.T) {
	def := Planer{Name: "Prüfungsplanung FK07", Email: "oliver.braun@hm.edu"}

	for _, tc := range []struct {
		name     string
		override *db.SemesterPlaner
	}{
		{"email without name", &db.SemesterPlaner{Email: strptr("vertretung@hm.edu")}},
		{"name without email", &db.SemesterPlaner{Name: strptr("Vertretung FK07")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := mergePlaner(def, tc.override)
			if got.Name != def.Name || got.Email != def.Email {
				t.Errorf("identity = %q/%q, want the configured one unchanged", got.Name, got.Email)
			}
			if !got.Inherited {
				t.Error("Inherited = false, want true")
			}
		})
	}
}
