package invigplan

// Violation describes a single broken constraint. For hard constraints the mere
// presence of a violation means the plan is infeasible; for soft constraints
// Penalty carries the (already weighted) cost contribution.
type Violation struct {
	Constraint    string
	Message       string
	InvigilatorID int // 0 for a global / position-level violation
	Day, Slot     int
	Penalty       float64
}

// HardConstraint must always hold. Check reports all violations in a (possibly
// externally built) plan – used by the validation. Allows is the fast
// feasibility test the optimizer uses to keep every plan hard-feasible by
// construction: it answers "may invigID take position posIdx in this plan?".
type HardConstraint interface {
	Name() string
	Check(p *Problem, plan *Plan) []Violation
	Allows(p *Problem, plan *Plan, posIdx, invigID int) bool
}

// SoftConstraint should hold. Cost returns the total (weighted) penalty of the
// plan together with the individual violations for reporting.
type SoftConstraint interface {
	Name() string
	Cost(p *Problem, plan *Plan) (penalty float64, violations []Violation)
}

// Registry bundles the active constraints. Add a new rule by appending it here
// (see DefaultRegistry). Both the optimizer and the validation use a Registry.
type Registry struct {
	Hard []HardConstraint
	Soft []SoftConstraint
}

// DefaultRegistry returns the constraints in their intended order.
func DefaultRegistry() *Registry {
	return &Registry{
		Hard: []HardConstraint{
			availabilityHard{},
			timeWindowHard{},
			ownExamHard{},
			oneInvigilationPerSlotHard{},
			timeGapHard{},
		},
		Soft: []SoftConstraint{
			minuteBalanceSoft{},
			coverageSoft{},
			maxDaysSoft{},
			preferOwnExamDaysSoft{},
			distributionSoft{kind: KindReserve},
			distributionSoft{kind: KindNTA},
			daySpanSoft{},
		},
	}
}

// Allows reports whether assigning posIdx to invigID violates no hard
// constraint. A fixed position can only keep its locked invigilator.
func (r *Registry) Allows(p *Problem, plan *Plan, posIdx, invigID int) bool {
	if locked, ok := p.Fixed[posIdx]; ok {
		return locked == invigID
	}
	for _, c := range r.Hard {
		if !c.Allows(p, plan, posIdx, invigID) {
			return false
		}
	}
	return true
}

// HardViolations returns all hard-constraint violations of the current plan.
func (r *Registry) HardViolations(p *Problem, plan *Plan) []Violation {
	var vs []Violation
	for _, c := range r.Hard {
		vs = append(vs, c.Check(p, plan)...)
	}
	return vs
}

// Cost is the optimizer's objective: the summed (weighted) soft penalty. The
// per-constraint penalties and violations are returned for reporting.
func (r *Registry) Cost(p *Problem, plan *Plan) (total float64, byConstraint map[string]float64, violations []Violation) {
	byConstraint = make(map[string]float64, len(r.Soft))
	for _, c := range r.Soft {
		penalty, vs := c.Cost(p, plan)
		total += penalty
		byConstraint[c.Name()] = penalty
		violations = append(violations, vs...)
	}
	return total, byConstraint, violations
}

// BalanceSatisfied reports whether every invigilator is within the tolerance of
// their target minutes. This is the optimizer's primary stopping criterion.
func (p *Problem) BalanceSatisfied(plan *Plan) bool {
	for i := range p.Invigilators {
		in := &p.Invigilators[i]
		dev := plan.DoingMinutes(in.ID) - in.TargetMinutes
		if dev < 0 {
			dev = -dev
		}
		if dev > p.ToleranceMin {
			return false
		}
	}
	return true
}

// Weights scales the soft-constraint penalties. They are tiered so that
// "minimum total cost" matches the intended priority order:
//
//	Coverage  ≫  BeyondTolerance  ≫  { Distribution, MaxDays, DaySpan, PreferExamDays }
//
// Coverage (filling every position) is near-mandatory. BeyondTolerance makes the
// primary objective – everyone within ±tolerance of their target minutes –
// dominate all remaining soft goals: it is applied linearly per minute outside
// the tolerance, with a weight large enough that a single person outside the
// band outranks the entire budget of the lower-tier constraints. MinuteBalance
// is the centering *inside* the band; it is scaled by the *relative* deviation
// (deviation / target) so that people who only have to do little are pulled
// closer to their target than people with a large workload. It is weighted
// *above* the distribution/days/span constraints, so getting the low-workload
// people close to zero takes priority over an even reserve/NTA distribution.
//
// Tier order: Coverage ≫ BeyondTolerance ≫ MinuteBalance ≫ {Distribution,
// MaxDays, DaySpan, PreferExamDays}.
type Weights struct {
	MinuteBalance   float64 // centering inside the band, scaled by relative deviation
	BeyondTolerance float64 // linear, per minute outside the band (dominant)
	Coverage        float64
	MaxDays         float64
	PreferExamDays  float64
	Distribution    float64
	DaySpan         float64

	// OverTargetFactor multiplies the centering penalty when a person is *over*
	// their target (doing more than they have to – "noch offen" negative); >1
	// prefers leaving someone slightly under their target to pushing them over,
	// especially for low-workload invigilators.
	OverTargetFactor float64
}

// DefaultWeights returns sensible starting weights; they are meant to be
// overridable from config (invigilation.optimizer.weights.*).
func DefaultWeights() Weights {
	return Weights{
		MinuteBalance:    10_000.0,
		BeyondTolerance:  10_000_000.0,
		Coverage:         100_000_000_000.0,
		MaxDays:          500.0,
		PreferExamDays:   50.0,
		Distribution:     200.0,
		DaySpan:          100.0,
		OverTargetFactor: 2.0,
	}
}
