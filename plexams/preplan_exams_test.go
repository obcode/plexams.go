package plexams

import "testing"

// TestPreplanInputDuration pins the creation default: a pre-exam entered without a duration
// is stored with the standard 90 minutes instead of being left open (which would make the
// booking-window gate work on a derived guess).
func TestPreplanInputDuration(t *testing.T) {
	tests := []struct {
		name    string
		entered *int
		want    int
	}{
		{"nothing entered → 90", nil, preplanDefaultDurationMinutes},
		{"zero → 90", min2(0), preplanDefaultDurationMinutes},
		{"negative → 90", min2(-30), preplanDefaultDurationMinutes},
		{"entered value kept", min2(120), 120},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := preplanInputDuration(tt.entered)
			if got == nil {
				t.Fatal("preplanInputDuration() = nil, want a duration")
			}
			if *got != tt.want {
				t.Errorf("preplanInputDuration() = %d, want %d", *got, tt.want)
			}
		})
	}
}
