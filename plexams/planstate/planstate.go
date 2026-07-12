// Package planstate is the generic planning-state engine: a 1-safe condition/event Petri
// net where each condition is a milestone that can be set automatically (Mark) or by hand
// (SetCondition), and a condition with a gate locks the matching generation while it is
// set. The concrete net (which conditions/phases/gates exist) is policy and is injected by
// the caller (plexams); this package only implements the mechanism over a small DB
// interface (satisfied by *db.DB).
package planstate

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// PhaseDef declares one planning phase.
type PhaseDef struct {
	Key   string
	Title string
}

// CondDef declares one planning condition (milestone). Gate, if not empty, is the area
// locked while this condition is set.
type CondDef struct {
	Key   string
	Title string
	Phase string
	Gate  model.PlanningGate
	// Compute, if set, makes this a derived condition: its Done state is computed live
	// from this predicate on every State() read instead of read from (and stored in) the
	// DB. Such a condition is read-only — it cannot be set or cleared by hand and follows
	// the underlying data automatically. A predicate error is logged and treated as "not
	// done" so the state assembly never fails on it.
	Compute func(ctx context.Context) (bool, error)
}

// DB is the persistence the engine needs; *db.DB satisfies it.
type DB interface {
	PlanningConditionsSet(ctx context.Context) ([]string, error)
	SetPlanningCondition(ctx context.Context, key string, set bool) error
}

// Machine is a planning-state engine over an injected net.
type Machine struct {
	db     DB
	phases []PhaseDef
	conds  []CondDef
}

// New builds the engine for the given net.
func New(db DB, phases []PhaseDef, conds []CondDef) *Machine {
	return &Machine{db: db, phases: phases, conds: conds}
}

func (m *Machine) condDefByKey(key string) (CondDef, bool) {
	for _, def := range m.conds {
		if def.Key == key {
			return def, true
		}
	}
	return CondDef{}, false
}

// State assembles the current planning state from the declarative net and the conditions
// stored as set in the DB.
func (m *Machine) State(ctx context.Context) (*model.PlanningState, error) {
	setKeys, err := m.db.PlanningConditionsSet(ctx)
	if err != nil {
		return nil, err
	}
	done := make(map[string]bool, len(setKeys))
	for _, key := range setKeys {
		done[key] = true
	}

	phaseByKey := make(map[string]*model.PlanningPhase)
	phases := make([]*model.PlanningPhase, 0, len(m.phases))
	for _, pd := range m.phases {
		phase := &model.PlanningPhase{Key: pd.Key, Title: pd.Title, Conditions: []*model.PlanningCondition{}}
		phaseByKey[pd.Key] = phase
		phases = append(phases, phase)
	}

	blocked := make([]model.PlanningGate, 0)
	for _, cd := range m.conds {
		isDone := done[cd.Key]
		if cd.Compute != nil {
			computed, err := cd.Compute(ctx)
			if err != nil {
				log.Error().Err(err).Str("key", cd.Key).
					Msg("cannot compute planning condition; treating as not done")
				computed = false
			}
			isDone = computed
		}
		cond := &model.PlanningCondition{
			Key:   cd.Key,
			Title: cd.Title,
			Phase: cd.Phase,
			Done:  isDone,
			Auto:  cd.Compute != nil,
		}
		if cd.Gate != "" {
			gate := cd.Gate
			cond.Gate = &gate
			if cond.Done {
				blocked = append(blocked, gate)
			}
		}
		if phase, ok := phaseByKey[cd.Phase]; ok {
			phase.Conditions = append(phase.Conditions, cond)
		}
	}

	return &model.PlanningState{Phases: phases, BlockedAreas: blocked}, nil
}

// SetCondition sets or clears a condition by hand. Errors on an unknown key.
func (m *Machine) SetCondition(ctx context.Context, key string, done bool) (*model.PlanningState, error) {
	def, ok := m.condDefByKey(key)
	if !ok {
		return nil, fmt.Errorf("unknown planning condition %q", key)
	}
	if def.Compute != nil {
		return nil, fmt.Errorf("planning condition %q is computed automatically and cannot be set by hand", key)
	}
	if err := m.db.SetPlanningCondition(ctx, key, done); err != nil {
		return nil, err
	}
	return m.State(ctx)
}

// Mark sets a condition as done. Best-effort: a failure is logged but never fails the
// operation that triggered it.
func (m *Machine) Mark(ctx context.Context, key string) {
	if err := m.db.SetPlanningCondition(ctx, key, true); err != nil {
		log.Error().Err(err).Str("key", key).Msg("cannot auto-mark planning condition")
	}
}

// Unmark clears a condition. Best-effort like Mark.
func (m *Machine) Unmark(ctx context.Context, key string) {
	if err := m.db.SetPlanningCondition(ctx, key, false); err != nil {
		log.Error().Err(err).Str("key", key).Msg("cannot auto-unmark planning condition")
	}
}

// EmailSendAllowed enforces that a "send once" email is sent at most once: while its
// condition is set, a real send (run==true) is refused; a dry run (run==false) is always
// allowed. On a successful real send the caller marks the condition. To resend, the
// condition has to be unset by hand.
func (m *Machine) EmailSendAllowed(ctx context.Context, condKey string, run bool) error {
	if !run {
		return nil
	}
	setKeys, err := m.db.PlanningConditionsSet(ctx)
	if err != nil {
		return err
	}
	for _, key := range setKeys {
		if key == condKey {
			def, _ := m.condDefByKey(condKey)
			return fmt.Errorf("email already sent (%s: %s); unset the planning condition to send it again",
				condKey, def.Title)
		}
	}
	return nil
}

// GenerationAllowed reports whether generation for the given area is allowed, i.e. no gate
// condition for that area is set. Returns an error describing the lock otherwise.
func (m *Machine) GenerationAllowed(ctx context.Context, area model.PlanningGate) error {
	setKeys, err := m.db.PlanningConditionsSet(ctx)
	if err != nil {
		return err
	}
	set := make(map[string]bool, len(setKeys))
	for _, key := range setKeys {
		set[key] = true
	}
	for _, cd := range m.conds {
		if cd.Gate == area && set[cd.Key] {
			return fmt.Errorf("%s is published (%s); generation is locked. "+
				"Make explicit changes instead, or unset the condition to regenerate", cd.Title, cd.Key)
		}
	}
	return nil
}
