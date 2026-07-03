package planstate

import (
	"context"
	"testing"

	"github.com/obcode/plexams.go/graph/model"
)

// fakeDB is an in-memory planstate.DB for testing the engine without a database.
type fakeDB struct {
	set map[string]bool
	err error
}

func newFakeDB(setKeys ...string) *fakeDB {
	f := &fakeDB{set: map[string]bool{}}
	for _, k := range setKeys {
		f.set[k] = true
	}
	return f
}

func (f *fakeDB) PlanningConditionsSet(_ context.Context) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	keys := make([]string, 0, len(f.set))
	for k, v := range f.set {
		if v {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

func (f *fakeDB) SetPlanningCondition(_ context.Context, key string, set bool) error {
	if f.err != nil {
		return f.err
	}
	f.set[key] = set
	return nil
}

var testPhases = []PhaseDef{{Key: "p0", Title: "Phase 0"}, {Key: "p1", Title: "Phase 1"}}
var testConds = []CondDef{
	{Key: "imported", Title: "Imported", Phase: "p0"},
	{Key: "published", Title: "Published", Phase: "p1", Gate: model.PlanningGateExams},
}

func TestStateAssemblesPhasesAndConditions(t *testing.T) {
	m := New(newFakeDB("imported"), testPhases, testConds)
	st, err := m.State(context.Background())
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if len(st.Phases) != 2 {
		t.Fatalf("got %d phases, want 2", len(st.Phases))
	}
	if len(st.Phases[0].Conditions) != 1 || !st.Phases[0].Conditions[0].Done {
		t.Errorf("p0 condition should be done")
	}
	if st.Phases[1].Conditions[0].Done {
		t.Errorf("p1 condition should not be done")
	}
	// no gate set -> no blocked area
	if len(st.BlockedAreas) != 0 {
		t.Errorf("got blocked %v, want none", st.BlockedAreas)
	}
}

func TestStateBlocksAreaWhenGateConditionSet(t *testing.T) {
	m := New(newFakeDB("published"), testPhases, testConds)
	st, err := m.State(context.Background())
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if len(st.BlockedAreas) != 1 || st.BlockedAreas[0] != model.PlanningGateExams {
		t.Errorf("got blocked %v, want [%s]", st.BlockedAreas, model.PlanningGateExams)
	}
}

func TestSetConditionUnknownKey(t *testing.T) {
	m := New(newFakeDB(), testPhases, testConds)
	if _, err := m.SetCondition(context.Background(), "nope", true); err == nil {
		t.Error("expected error for unknown condition key")
	}
}

func TestEmailSendAllowed(t *testing.T) {
	ctx := context.Background()
	m := New(newFakeDB("imported"), testPhases, testConds)

	// dry run always allowed, even when the condition is set
	if err := m.EmailSendAllowed(ctx, "imported", false); err != nil {
		t.Errorf("dry run should always be allowed: %v", err)
	}
	// real send refused while the condition is set
	if err := m.EmailSendAllowed(ctx, "imported", true); err == nil {
		t.Error("real send should be refused while condition is set")
	}
	// real send allowed when the condition is not set
	if err := m.EmailSendAllowed(ctx, "published", true); err != nil {
		t.Errorf("real send should be allowed when condition unset: %v", err)
	}
}

func TestGenerationAllowed(t *testing.T) {
	ctx := context.Background()

	// gate condition set -> generation for that area locked
	if err := New(newFakeDB("published"), testPhases, testConds).
		GenerationAllowed(ctx, model.PlanningGateExams); err == nil {
		t.Error("generation should be locked while the gate condition is set")
	}
	// gate condition not set -> allowed
	if err := New(newFakeDB(), testPhases, testConds).
		GenerationAllowed(ctx, model.PlanningGateExams); err != nil {
		t.Errorf("generation should be allowed when gate condition unset: %v", err)
	}
}

func TestMarkAndUnmark(t *testing.T) {
	ctx := context.Background()
	db := newFakeDB()
	m := New(db, testPhases, testConds)

	m.Mark(ctx, "imported")
	if !db.set["imported"] {
		t.Error("Mark should set the condition")
	}
	m.Unmark(ctx, "imported")
	if db.set["imported"] {
		t.Error("Unmark should clear the condition")
	}
}
