package obs

import (
	"strings"
	"testing"
)

const testKey = "not-a-real-key-just-for-this-test"

func TestPseudonymIsStableAndKeyed(t *testing.T) {
	a := pseudonym(testMail, testKey)
	if a == "" {
		t.Fatal("no pseudonym for a configured key")
	}
	if len(a) != pseudonymLength {
		t.Errorf("length = %d, want %d", len(a), pseudonymLength)
	}
	if got := pseudonym(testMail, testKey); got != a {
		t.Errorf("not stable: %q then %q", a, got)
	}
	if got := pseudonym("someone.else@hm.edu", testKey); got == a {
		t.Error("two addresses share a pseudonym")
	}
	// Rotating the key renumbers everyone -- new users, not wrong ones.
	if got := pseudonym(testMail, testKey+"-rotated"); got == a {
		t.Error("the key does not affect the pseudonym")
	}
	if strings.Contains(a, "@") || strings.Contains(a, "hm.edu") {
		t.Errorf("pseudonym looks like an address: %q", a)
	}
}

func TestPseudonymNeedsAKey(t *testing.T) {
	if got := pseudonym(testMail, ""); got != "" {
		t.Errorf("pseudonym without a key = %q, want none", got)
	}
}

func TestUserForSendsNothingWithoutAKey(t *testing.T) {
	// Without a key the choice is between an unkeyed hash of an address drawn
	// from a small, guessable directory and no user at all. It must be the
	// latter.
	restore := setReportCfg("", false)
	defer restore()

	if _, ok := userFor(testMail); ok {
		t.Error("a user was built without a key")
	}
}

func TestUserForPseudonymises(t *testing.T) {
	restore := setReportCfg(testKey, false)
	defer restore()

	user, ok := userFor("  " + strings.ToUpper(testMail) + " ")
	if !ok {
		t.Fatal("no user built")
	}
	if user.Email != "" || user.Name != "" || user.Username != "" {
		t.Errorf("personal fields set: %+v", user)
	}
	// Case and padding must not create a second identity for one person.
	if want := pseudonym(testMail, testKey); user.ID != want {
		t.Errorf("id = %q, want %q", user.ID, want)
	}
}

func TestUserForHonoursTheOperatorOptIn(t *testing.T) {
	restore := setReportCfg(testKey, true)
	defer restore()

	user, ok := userFor(testMail)
	if !ok || user.Email != testMail {
		t.Errorf("user = %+v, ok = %v, want the address", user, ok)
	}
}

func TestUserForIgnoresAnEmptyAddress(t *testing.T) {
	restore := setReportCfg(testKey, false)
	defer restore()

	if _, ok := userFor("   "); ok {
		t.Error("a user was built from an empty address")
	}
}

// setReportCfg installs a configuration and returns a function restoring the
// previous one.
func setReportCfg(key string, sendEmail bool) func() {
	previous := reportCfg
	reportCfg.userKey = key
	reportCfg.sendUserEmail = sendEmail
	return func() { reportCfg = previous }
}
