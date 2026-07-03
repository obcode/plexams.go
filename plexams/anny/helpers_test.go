package anny

import "testing"

// Characterization tests for the pure Anny helpers, moved here with the code they cover
// when the Anny integration was extracted from the plexams package.

func TestMatchesAnyPersonalization(t *testing.T) {
	tests := []struct {
		name  string
		value string
		names []string
		want  bool
	}{
		{"no names configured matches everything", "Braun", nil, true},
		{"empty names slice matches everything", "Braun", []string{}, true},
		{"exact match", "Braun", []string{"Braun"}, true},
		{"case insensitive", "braun", []string{"BRAUN"}, true},
		{"trims the value", "  Braun  ", []string{"Braun"}, true},
		{"matches any of several", "Meier", []string{"Braun", "Meier"}, true},
		{"no match", "Schmidt", []string{"Braun", "Meier"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchesAnyPersonalization(tt.value, tt.names); got != tt.want {
				t.Errorf("MatchesAnyPersonalization(%q, %v) = %v, want %v", tt.value, tt.names, got, tt.want)
			}
		})
	}
}

func TestIsApprovedStatus(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{"accepted", true},
		{"acceptet", true}, // known typo in upstream data, deliberately accepted
		{"ACCEPTED", true},
		{"  Accepted  ", true},
		{"pending", false},
		{"rejected", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsApprovedStatus(tt.status); got != tt.want {
			t.Errorf("IsApprovedStatus(%q) = %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestNormalizeRoomName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"R1.006", "R1.006"},
		{" r1.006 ", "R1.006"},
		{"T 3.014", "T3.014"},
		{"  a b  c ", "ABC"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := normalizeRoomName(tt.in); got != tt.want {
			t.Errorf("normalizeRoomName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
