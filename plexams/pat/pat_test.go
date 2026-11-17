package pat

import (
	"errors"
	"regexp"
	"strings"
	"testing"
)

// The shape the database CHECK and the gitleaks rule rely on.
var tokenShape = regexp.MustCompile(`^plexams_[0-9A-HJKMNP-TV-Z]{16}_[A-Za-z0-9_-]{43}$`)

func TestMintParseRoundTrip(t *testing.T) {
	seen := map[string]bool{}
	for range 200 {
		m, err := Mint()
		if err != nil {
			t.Fatal(err)
		}
		if !tokenShape.MatchString(m.Plaintext) {
			t.Fatalf("minted token has the wrong shape: %q", m.Plaintext)
		}
		if seen[m.ID] {
			t.Fatal("duplicate id")
		}
		seen[m.ID] = true

		p, err := Parse(m.Plaintext)
		if err != nil || p.ID != m.ID {
			t.Fatalf("Parse(%q) = %+v, %v", m.Plaintext, p, err)
		}
		if !Matches(p.Secret, m.SecretHash) {
			t.Fatal("the minted secret must match its hash")
		}
	}
}

func TestParseRejectsEverythingElse(t *testing.T) {
	m, _ := Mint()
	bad := []string{
		"",
		"tallox_" + strings.TrimPrefix(m.Plaintext, "plexams_"),
		strings.Replace(m.Plaintext, "_", "-", 2),
		m.Plaintext + "x",
		m.Plaintext[:len(m.Plaintext)-1],
		"plexams_" + strings.Repeat("I", 16) + "_" + strings.Repeat("A", 43), // I is not Crockford
		"plexams_" + strings.Repeat("A", 16) + "_" + strings.Repeat("!", 43),
	}
	for _, b := range bad {
		if _, err := Parse(b); !errors.Is(err, ErrMalformed) {
			t.Errorf("Parse(%q) must fail with ErrMalformed, got %v", b, err)
		}
	}
	if _, err := Parse("  " + m.Plaintext + " "); err != nil {
		t.Errorf("surrounding whitespace is tolerated, got %v", err)
	}
}

func TestMatchesRefusesWrongAndMissing(t *testing.T) {
	m, _ := Mint()
	other, _ := Mint()
	p, _ := Parse(other.Plaintext)
	if Matches(p.Secret, m.SecretHash) {
		t.Error("another token's secret must not match")
	}
	if Matches(p.Secret, nil) {
		t.Error("no stored hash never matches")
	}
}
