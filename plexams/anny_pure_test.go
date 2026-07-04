package plexams

import (
	"testing"
	"time"

	"github.com/obcode/plexams.go/graph/model"
)

// These are characterization tests: they pin down the CURRENT behaviour of the shared
// Anny/room booking helpers before the plexams package is decomposed, so a later refactor
// that moves this logic into its own package cannot change it unnoticed.

func TestSlotBlockDuration(t *testing.T) {
	tests := []struct {
		name       string
		starttimes []*model.Starttime
		want       time.Duration
	}{
		{"nil defaults to 120min", nil, 120 * time.Minute},
		{"single starttime defaults to 120min", []*model.Starttime{{Start: "08:30"}}, 120 * time.Minute},
		{"two starttimes give their difference", []*model.Starttime{{Start: "08:30"}, {Start: "10:30"}}, 120 * time.Minute},
		{"90 minute spacing", []*model.Starttime{{Start: "08:00"}, {Start: "09:30"}}, 90 * time.Minute},
		{"only first two matter", []*model.Starttime{{Start: "08:00"}, {Start: "10:00"}, {Start: "14:00"}}, 120 * time.Minute},
		{"unparseable falls back to 120min", []*model.Starttime{{Start: "foo"}, {Start: "bar"}}, 120 * time.Minute},
		{"non-increasing falls back to 120min", []*model.Starttime{{Start: "10:00"}, {Start: "08:00"}}, 120 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := slotBlockDuration(tt.starttimes); got != tt.want {
				t.Errorf("slotBlockDuration() = %v, want %v", got, tt.want)
			}
		})
	}
}
