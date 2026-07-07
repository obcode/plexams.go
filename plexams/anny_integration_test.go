package plexams

import (
	"context"
	"testing"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/mongotest"
	"github.com/obcode/plexams.go/plexams/anny"
)

// Characterization tests for the DB-backed Anny booking logic that feeds the Terminplan
// generator (annyBookedBySlot) and the room-request views (ExahmRoomsFromAnnyBookings).
// They run against an ephemeral MongoDB (testcontainers or PLEXAMS_TEST_MONGO_URI) and are
// skipped when neither is available. Pinned before decomposing the plexams package.

func intPtr(i int) *int { return &i }

// annyTestPlexams builds a *Plexams wired to a throwaway DB with one two-hour slot
// (day 1 / slot 1) starting 2026-01-20 08:30, so a booking window can cover it.
func annyTestPlexams(t *testing.T) (*Plexams, context.Context, time.Time) {
	t.Helper()
	dbClient := mongotest.NewDB(t)
	ctx := context.Background()

	slotStart := time.Date(2026, 1, 20, 8, 30, 0, 0, time.Local)
	p := &Plexams{
		dbClient: dbClient,
		semesterConfig: &model.SemesterConfig{
			Starttimes: []*model.Starttime{{Start: "08:30"}, {Start: "10:30"}},
		},
		allSlots: []*model.Slot{{Starttime: slotStart}},
	}
	p.anny = anny.New(dbClient, anny.Config{})
	return p, ctx, slotStart
}

func TestAnnyBookedBySlot(t *testing.T) {
	p, ctx, slotStart := annyTestPlexams(t)

	// Rooms master data: one Anny EXaHM room, one Anny SEB room (reduced SEB seats),
	// one non-Anny room (must be ignored), one deactivated Anny room (must be ignored).
	for _, r := range []*model.Room{
		{Name: "T3.014", Seats: 30, RequestWith: model.RoomRequestTypeAnny, Exahm: true},
		{Name: "T3.015", Seats: 40, RequestWith: model.RoomRequestTypeAnny, Seb: true, SebSeats: intPtr(20)},
		{Name: "R1.006", Seats: 50, RequestWith: model.RoomRequestTypeNone, Exahm: true},
		{Name: "T3.099", Seats: 60, RequestWith: model.RoomRequestTypeAnny, Exahm: true, Deactivated: true},
	} {
		if _, err := p.dbClient.AddRoom(ctx, r); err != nil {
			t.Fatalf("AddRoom(%s): %v", r.Name, err)
		}
	}

	if err := p.dbClient.SetAnnyConfig(ctx, &model.AnnyConfig{PersonalizationNames: []string{"Braun"}}); err != nil {
		t.Fatalf("SetAnnyConfig: %v", err)
	}

	covers := func(room, who string) *model.AnnyBooking {
		return &model.AnnyBooking{
			Room:                room,
			PersonalizationName: who,
			StartDate:           slotStart.Add(-30 * time.Minute), // 08:00
			EndDate:             slotStart.Add(150 * time.Minute), // 11:00, covers 08:30..10:30
			Status:              "accepted",
		}
	}
	bookings := []*model.AnnyBooking{
		covers("T3.014", "Braun"), // ours, EXaHM room -> counts
		covers("T3.015", "Braun"), // ours, SEB room -> counts SEB seats
		covers("R1.006", "Braun"), // ours but not an Anny room -> ignored
		covers("T3.099", "Braun"), // ours, Anny, but deactivated -> ignored
		covers("T3.014", "Meier"), // someone else's -> ignored
		{ // ours but window ends before the slot block finishes -> ignored
			Room: "T3.014", PersonalizationName: "Braun",
			StartDate: slotStart.Add(-30 * time.Minute), EndDate: slotStart.Add(60 * time.Minute), // ends 09:30
			Status: "accepted",
		},
	}
	if err := p.dbClient.SaveAnnyBookings(ctx, bookings); err != nil {
		t.Fatalf("SaveAnnyBookings: %v", err)
	}

	got, err := p.annyBookedByTime(ctx, []time.Time{slotStart})
	if err != nil {
		t.Fatalf("annyBookedByTime: %v", err)
	}
	sb := got[slotStart]
	if sb == nil {
		t.Fatal("no slotBooking for the 08:30 slot")
	}
	if sb.exahmSeats != 30 {
		t.Errorf("exahmSeats = %d, want 30 (only T3.014)", sb.exahmSeats)
	}
	if sb.sebSeats != 20 {
		t.Errorf("sebSeats = %d, want 20 (T3.015 reduced SEB seats)", sb.sebSeats)
	}
	if sb.seats != 70 {
		t.Errorf("seats = %d, want 70 (T3.014 30 + T3.015 40)", sb.seats)
	}
	if !sb.rooms["T3.014"] || !sb.rooms["T3.015"] {
		t.Errorf("rooms = %v, want T3.014 and T3.015 booked", sb.rooms)
	}
	if sb.rooms["R1.006"] || sb.rooms["T3.099"] {
		t.Errorf("rooms = %v, must not include non-Anny/deactivated rooms", sb.rooms)
	}
}

func TestAnnyBookedBySlotEmptyKeys(t *testing.T) {
	p, ctx, _ := annyTestPlexams(t)
	got, err := p.annyBookedByTime(ctx, nil)
	if err != nil {
		t.Fatalf("annyBookedByTime(nil): %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d entries for no keys, want 0", len(got))
	}
}

func TestExahmRoomsFromAnnyBookings(t *testing.T) {
	p, ctx, slotStart := annyTestPlexams(t)

	if _, err := p.dbClient.AddRoom(ctx, &model.Room{Name: "T3.014", Seats: 30, RequestWith: model.RoomRequestTypeAnny, Exahm: true}); err != nil {
		t.Fatalf("AddRoom: %v", err)
	}
	if err := p.dbClient.SetAnnyConfig(ctx, &model.AnnyConfig{PersonalizationNames: []string{"Braun"}}); err != nil {
		t.Fatalf("SetAnnyConfig: %v", err)
	}
	if err := p.dbClient.SaveAnnyBookings(ctx, []*model.AnnyBooking{
		{Room: "T3.014", PersonalizationName: "Braun", StartDate: slotStart, EndDate: slotStart.Add(2 * time.Hour), Status: "accepted"},
		{Room: "T3.014", PersonalizationName: "Meier", StartDate: slotStart, EndDate: slotStart.Add(2 * time.Hour), Status: "accepted"}, // not ours
		{Room: "R9.999", PersonalizationName: "Braun", StartDate: slotStart, EndDate: slotStart.Add(2 * time.Hour), Status: "accepted"}, // unknown room
	}); err != nil {
		t.Fatalf("SaveAnnyBookings: %v", err)
	}

	entries, err := p.ExahmRoomsFromAnnyBookings(ctx)
	if err != nil {
		t.Fatalf("ExahmRoomsFromAnnyBookings: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1 (only our T3.014 booking)", len(entries))
	}
	if len(entries[0].Rooms) != 1 || entries[0].Rooms[0] != "T3.014" {
		t.Errorf("entry rooms = %v, want [T3.014]", entries[0].Rooms)
	}
	if !entries[0].Approved {
		t.Errorf("entry should be approved (status accepted)")
	}
}

func TestAnnyConfigRoundTrip(t *testing.T) {
	p, ctx, _ := annyTestPlexams(t)

	// Nothing stored yet: AnnyConfig returns the (empty) config-file seed, never nil names.
	cfg, err := p.AnnyConfig(ctx)
	if err != nil {
		t.Fatalf("AnnyConfig: %v", err)
	}
	if cfg == nil || cfg.PersonalizationNames == nil {
		t.Fatalf("AnnyConfig with nothing stored = %+v, want non-nil names", cfg)
	}

	// Setting trims blanks and drops empties.
	set, err := p.SetAnnyPersonalizationNames(ctx, []string{" Braun ", "", "Meier"})
	if err != nil {
		t.Fatalf("SetAnnyPersonalizationNames: %v", err)
	}
	if len(set.PersonalizationNames) != 2 || set.PersonalizationNames[0] != "Braun" || set.PersonalizationNames[1] != "Meier" {
		t.Fatalf("stored names = %v, want [Braun Meier]", set.PersonalizationNames)
	}

	names := p.anny.PersonalizationNames(ctx)
	if len(names) != 2 {
		t.Errorf("PersonalizationNames = %v, want 2 names", names)
	}
}

func TestMarkMineAnnyBookings(t *testing.T) {
	p, ctx, slotStart := annyTestPlexams(t)
	if err := p.dbClient.SetAnnyConfig(ctx, &model.AnnyConfig{PersonalizationNames: []string{"Braun"}}); err != nil {
		t.Fatalf("SetAnnyConfig: %v", err)
	}
	if err := p.dbClient.SaveAnnyBookings(ctx, []*model.AnnyBooking{
		{Room: "T3.014", PersonalizationName: "Braun", StartDate: slotStart, EndDate: slotStart.Add(time.Hour), Status: "accepted"},
		{Room: "T3.014", PersonalizationName: "Meier", StartDate: slotStart, EndDate: slotStart.Add(time.Hour), Status: "accepted"},
	}); err != nil {
		t.Fatalf("SaveAnnyBookings: %v", err)
	}

	bookings, err := p.AllAnnyBookings(ctx)
	if err != nil {
		t.Fatalf("AllAnnyBookings: %v", err)
	}
	mine := 0
	for _, b := range bookings {
		if b.Mine {
			mine++
		}
	}
	if mine != 1 {
		t.Errorf("got %d mine=true bookings, want 1 (Braun)", mine)
	}
}
