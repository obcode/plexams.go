package anny

import (
	"strings"
	"testing"
	"time"

	"github.com/obcode/plexams.go/graph/model"
)

// exam period covering the whole of March 2026.
var (
	periodFrom  = time.Date(2026, 3, 1, 0, 0, 0, 0, time.Local)
	periodUntil = time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local)
	testWindow  = DiffWindow{From: periodFrom, Until: periodUntil}
	testNow     = time.Date(2026, 3, 1, 12, 0, 0, 0, time.Local)
	myNames     = []string{"Braun"}
)

func mkBooking(number, room string, start time.Time, status, who string) *model.AnnyBooking {
	return &model.AnnyBooking{
		Number:              number,
		Room:                room,
		StartDate:           start,
		EndDate:             start.Add(90 * time.Minute),
		Status:              status,
		PersonalizationName: who,
	}
}

func TestDiffBookings(t *testing.T) {
	on := func(day int) time.Time { return time.Date(2026, 3, day, 9, 0, 0, 0, time.Local) }

	old := []*model.AnnyBooking{
		mkBooking("B1", "R1.001", on(5), "accepted", "Braun"),  // unchanged (mine)
		mkBooking("B2", "R1.002", on(6), "accepted", "Müller"), // status changes (fremd)
		mkBooking("B4", "R1.004", on(7), "accepted", "Braun"),  // dropped → removed
	}
	neu := []*model.AnnyBooking{
		mkBooking("B1", "R1.001", on(5), "accepted", "Braun"),  // unchanged
		mkBooking("B2", "R1.002", on(6), "rejected", "Müller"), // changed
		mkBooking("B5", "R1.005", on(8), "accepted", "Braun"),  // new → added
	}

	diff := diffBookings(old, neu, testWindow, testNow, myNames)
	if diff.entry.Added != 1 || diff.entry.Changed != 1 || diff.entry.Removed != 1 {
		t.Fatalf("counts = added %d changed %d removed %d, want 1/1/1",
			diff.entry.Added, diff.entry.Changed, diff.entry.Removed)
	}

	// mine-vs-others is marked, the booker is shown.
	var added *model.SyncChangeEntry
	for _, e := range diff.entry.Entries {
		if e.Type == "added" {
			added = e
		}
	}
	if added == nil || !strings.HasPrefix(added.Name, "[eigene]") || !strings.Contains(added.Name, "Braun") {
		t.Errorf("added entry name = %q, want [eigene]…Braun", added.Name)
	}

	var changed *model.SyncChangeEntry
	for _, e := range diff.entry.Entries {
		if e.Type == "changed" {
			changed = e
		}
	}
	if changed == nil || !strings.HasPrefix(changed.Name, "[fremd]") {
		t.Fatalf("changed entry = %+v, want a [fremd] entry", changed)
	}
	var statusChange *model.SyncFieldChange
	for _, f := range changed.Fields {
		if f.Field == "status" {
			statusChange = f
		}
	}
	if statusChange == nil || statusChange.Old != "accepted" || statusChange.New != "rejected" {
		t.Errorf("status change not recorded: %+v", changed.Fields)
	}
}

func TestDiffBookingsRestrictedToExamPeriod(t *testing.T) {
	beforePeriod := time.Date(2026, 2, 15, 9, 0, 0, 0, time.Local)
	afterPeriod := time.Date(2026, 4, 15, 9, 0, 0, 0, time.Local)
	lastDay := time.Date(2026, 3, 31, 14, 0, 0, 0, time.Local) // still inside the Until day

	old := []*model.AnnyBooking{
		mkBooking("BEFORE", "R1.001", beforePeriod, "accepted", "Braun"), // out of period → ignored
		mkBooking("AFTER", "R1.002", afterPeriod, "accepted", "Braun"),   // out of period → ignored
		mkBooking("LAST", "R1.003", lastDay, "accepted", "Braun"),        // dropped inside period → removed
	}
	neu := []*model.AnnyBooking{} // everything gone

	diff := diffBookings(old, neu, testWindow, testNow, myNames)
	// only the in-period LAST booking counts as removed; BEFORE/AFTER are outside.
	if diff.entry.Removed != 1 {
		t.Fatalf("removed = %d, want 1 (only in-period bookings)", diff.entry.Removed)
	}
	for _, e := range diff.entry.Entries {
		if strings.Contains(e.Name, "BEFORE") || strings.Contains(e.Name, "AFTER") {
			t.Errorf("out-of-period booking reported: %q", e.Name)
		}
	}
}

func TestDiffBookingsNoChanges(t *testing.T) {
	b := mkBooking("B1", "R1.001", time.Date(2026, 3, 10, 9, 0, 0, 0, time.Local), "accepted", "Braun")
	diff := diffBookings([]*model.AnnyBooking{b}, []*model.AnnyBooking{b}, testWindow, testNow, myNames)
	if diff.entry.Added+diff.entry.Changed+diff.entry.Removed != 0 {
		t.Errorf("expected no changes, got %+v", diff.entry)
	}
}
