package optimize

// Kind distinguishes hard (must always hold) from soft (weighted objective)
// constraints.
type Kind int

const (
	KindHard Kind = iota
	KindSoft
)

func (k Kind) String() string {
	if k == KindHard {
		return "hard"
	}
	return "soft"
}

// Info is a self-description of a constraint for reporting and the read-only
// "which constraints are applied" view in the GUI. It carries no evaluation
// logic; it only explains what the constraint does.
type Info struct {
	// Name is a stable machine key (used as the map key for per-constraint cost).
	Name string
	// Title is a short human-readable (German) label.
	Title string
	// Description explains what the constraint enforces or optimizes.
	Description string
	Kind        Kind
	// Weight is the soft-constraint weight (0 for hard constraints).
	Weight float64
	// Tier orders constraints by priority for display (lower = more important);
	// optional (0 if unused).
	Tier int
}

// Violation is a single broken constraint. For a hard constraint its presence means
// the state is infeasible; for a soft constraint Penalty is the (already weighted)
// cost contribution. Refs holds the involved entities (e.g. ancodes) for the UI.
type Violation struct {
	Constraint string
	Message    string
	Penalty    float64
	Refs       []int
}

// Describable is implemented by every constraint so it can be listed.
type Describable interface {
	Info() Info
}

// HardConstraint must always hold. Check reports all violations of a given state
// (used for validation and reporting). The fast per-move feasibility test the
// optimizer uses to keep every state hard-feasible is generator-specific (it needs
// the move type) and therefore lives in the Model, not here.
type HardConstraint[S any] interface {
	Describable
	Check(state S) []Violation
}

// SoftConstraint should hold. Cost returns the total (weighted) penalty of a state
// together with the individual violations for reporting.
type SoftConstraint[S any] interface {
	Describable
	Cost(state S) (penalty float64, violations []Violation)
}

// Registry bundles the active constraints for one generator over its state type S.
// It provides the soft objective (Cost), hard validation (HardViolations) and the
// read-only description (Describe).
type Registry[S any] struct {
	Hard []HardConstraint[S]
	Soft []SoftConstraint[S]
}

// Cost is the summed (weighted) soft penalty; the per-constraint breakdown (keyed by
// Info().Name) and the individual violations are returned for reporting.
func (r Registry[S]) Cost(state S) (total float64, byConstraint map[string]float64, violations []Violation) {
	byConstraint = make(map[string]float64, len(r.Soft))
	for _, c := range r.Soft {
		penalty, vs := c.Cost(state)
		total += penalty
		byConstraint[c.Info().Name] = penalty
		violations = append(violations, vs...)
	}
	return total, byConstraint, violations
}

// HardViolations returns all hard-constraint violations of the given state.
func (r Registry[S]) HardViolations(state S) []Violation {
	var vs []Violation
	for _, c := range r.Hard {
		vs = append(vs, c.Check(state)...)
	}
	return vs
}

// Describe lists every constraint (hard first, then soft) for the read-only view.
func (r Registry[S]) Describe() []Info {
	out := make([]Info, 0, len(r.Hard)+len(r.Soft))
	for _, c := range r.Hard {
		out = append(out, c.Info())
	}
	for _, c := range r.Soft {
		out = append(out, c.Info())
	}
	return out
}
