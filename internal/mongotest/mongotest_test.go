package mongotest_test

import (
	"context"
	"testing"

	"github.com/obcode/plexams.go/internal/mongotest"
)

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
