package mongotest_test

import (
	"context"
	"strings"
	"testing"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/mongotest"
)

// TestGlobalDatabaseIsIsolated is a regression guard. The master data (rooms, NTAs,
// study programs, …) lives in one database shared by every semester, so a test writing
// a fixture there lands in the *real* master data and later runs see the leftovers.
// That happened: it corrupted the room master data and made TestAnnyBookedBySlot flaky.
func TestGlobalDatabaseIsIsolated(t *testing.T) {
	d := mongotest.NewDB(t)
	ctx := context.Background()

	if name := d.GlobalDatabaseName(); !strings.HasPrefix(name, "plexams_test_") {
		t.Fatalf("global database is %q — tests must never write into the real master data", name)
	}

	// A room written here must not be visible to a second, independent test DB.
	if _, err := d.AddRoom(ctx, &model.Room{Name: "ZZ.999", Seats: 1}); err != nil {
		t.Fatalf("AddRoom: %v", err)
	}
	other := mongotest.NewDB(t)
	rooms, err := other.Rooms(ctx)
	if err != nil {
		t.Fatalf("Rooms: %v", err)
	}
	for _, r := range rooms {
		if r.Name == "ZZ.999" {
			t.Error("room leaked into another test's master data")
		}
	}
}

func TestHelperRoundTrip(t *testing.T) {
	d := mongotest.NewDB(t)
	ctx := context.Background()

	if err := d.SetPlanningCondition(ctx, "smokeTest", true); err != nil {
		t.Fatalf("set planning condition: %v", err)
	}
	set, err := d.PlanningConditionsSet(ctx)
	if err != nil {
		t.Fatalf("read planning conditions: %v", err)
	}
	found := false
	for _, k := range set {
		if k == "smokeTest" {
			found = true
		}
	}
	if !found {
		t.Errorf("condition not persisted, got %v", set)
	}
}
