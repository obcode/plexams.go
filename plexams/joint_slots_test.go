package plexams

import (
	"testing"
	"time"

	"github.com/obcode/plexams.go/graph/model"
)

func TestDeriveJointProgramSlots(t *testing.T) {
	at := func(day, hour, min int) time.Time {
		return time.Date(2026, 7, day, hour, min, 0, 0, time.Local)
	}
	// grid slots: two days × two start times
	slots := []*model.Slot{
		{Starttime: at(6, 8, 30)},
		{Starttime: at(6, 10, 30)},
		{Starttime: at(7, 8, 30)},
		{Starttime: at(7, 10, 30)},
	}

	jpts := []*model.JointProgramTimes{
		// DE reserves the two 08:30 slots
		{Program: "DE", AllowedTimes: []time.Time{at(6, 8, 30), at(7, 8, 30)}},
		// HE (MUC.HEALTH) reserves a single 10:30 slot + one time NOT on the grid (dropped)
		{Program: "HE", AllowedTimes: []time.Time{at(6, 10, 30), at(6, 12, 30)}},
		// a program with no reserved times → empty slot list
		{Program: "GS", AllowedTimes: nil},
	}

	got := deriveJointProgramSlots(slots, jpts)
	if len(got) != 3 {
		t.Fatalf("expected 3 program entries, got %d", len(got))
	}

	byProg := make(map[string][]time.Time)
	for _, jps := range got {
		for _, s := range jps.Slots {
			byProg[jps.Program] = append(byProg[jps.Program], s.Starttime)
		}
	}

	wantDE := []time.Time{at(6, 8, 30), at(7, 8, 30)}
	if !sameTimes(byProg["DE"], wantDE) {
		t.Errorf("DE slots = %v, want %v", byProg["DE"], wantDE)
	}
	// the off-grid 12:30 time is dropped, only the matching 10:30 slot remains
	wantHE := []time.Time{at(6, 10, 30)}
	if !sameTimes(byProg["HE"], wantHE) {
		t.Errorf("HE slots = %v, want %v", byProg["HE"], wantHE)
	}
	if len(byProg["GS"]) != 0 {
		t.Errorf("GS should have no reserved slots, got %v", byProg["GS"])
	}
}

func sameTimes(a, b []time.Time) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Equal(b[i]) {
			return false
		}
	}
	return true
}
