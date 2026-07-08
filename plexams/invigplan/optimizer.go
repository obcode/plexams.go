package invigplan

import (
	"math"
	"math/rand"
	"sort"

	"github.com/obcode/plexams.go/plexams/optimize"
)

// Options controls the simulated-annealing optimizer.
type Options struct {
	Iterations int
	StartTemp  float64
	EndTemp    float64
	Seed       int64

	// StopOnBalance allows the search to end early once the balance is reached
	// and everything is filled, but only after StagnationLimit iterations without
	// any further improvement of the best plan – so the remaining soft goals
	// (distribution, days, span) still get optimized once the balance holds.
	StopOnBalance   bool
	StagnationLimit int

	// OnProgress, if set, is called every ProgressEvery iterations with a
	// snapshot of the current best plan. It is throttled on purpose: calling it
	// per iteration would dominate the runtime with terminal I/O.
	OnProgress    func(Progress)
	ProgressEvery int
}

// Progress is a throttled snapshot of the optimizer state for UI feedback.
type Progress struct {
	Iteration int
	Total     int
	BestCost  float64
	Balance   bool
	Unfilled  int
}

// DefaultOptions returns sensible defaults. The temperatures are scaled to the
// typical lower-tier soft-constraint delta of a single move; the dominant
// beyond-tolerance penalty is large enough to be treated as near-hard.
func DefaultOptions() Options {
	return Options{
		Iterations:      1_000_000,
		StartTemp:       20_000,
		EndTemp:         0.5,
		Seed:            1,
		StopOnBalance:   true,
		StagnationLimit: 30_000,
	}
}

// Result reports what the optimizer achieved.
type Result struct {
	Cost             float64
	CostByConstraint map[string]float64
	Violations       []Violation
	BalanceSatisfied bool
	Unfilled         int
	Iterations       int
	StoppedEarly     bool
}

// change records a single (position -> invigilator) reassignment so a move can
// be undone after a rejected annealing step.
type change struct {
	pos      int
	oldInvig int
	newInvig int
}

// Optimize builds a greedy hard-feasible start and improves it with simulated
// annealing. Every intermediate plan stays hard-feasible (moves are only
// applied when the registry allows them), so the result satisfies all hard
// constraints; the soft constraints are traded off via the cost function.
func Optimize(p *Problem, reg *Registry, opts Options) (*Plan, Result) {
	rng := rand.New(rand.NewSource(opts.Seed)) //nolint:gosec // not security relevant
	plan := Greedy(p, reg, rng)

	movable := movablePositions(p)
	result := Result{Iterations: opts.Iterations}

	if len(movable) > 0 && len(p.Invigilators) > 0 {
		model := &invigModel{p: p, reg: reg, plan: plan, movable: movable}
		// The greedy start already drew from rng; share the same stream with the anneal
		// (opts.Rng) so the result is identical to the old inline loop.
		oopts := optimize.Options{
			Iterations:        opts.Iterations,
			StartTemp:         opts.StartTemp,
			EndTemp:           opts.EndTemp,
			Rng:               rng,
			StopWhenConverged: opts.StopOnBalance,
			StagnationLimit:   opts.StagnationLimit,
			ProgressEvery:     opts.ProgressEvery,
		}
		if opts.OnProgress != nil && opts.ProgressEvery > 0 {
			oopts.OnProgress = func(pr optimize.Progress) {
				// model.bestBalance/bestUnfilled are refreshed in Converged(), which the
				// engine calls exactly when the best plan is (re)snapshotted.
				opts.OnProgress(Progress{
					Iteration: pr.Iteration, Total: pr.Total, BestCost: pr.BestCost,
					Balance: model.bestBalance, Unfilled: model.bestUnfilled,
				})
			}
		}
		res := optimize.Anneal(model, oopts)
		plan = model.plan // Anneal restores the model to the best plan found
		result.Iterations = res.Iterations
		result.StoppedEarly = res.StoppedEarly
	}

	total, byConstraint, violations := reg.Cost(p, plan)
	result.Cost = total
	result.CostByConstraint = byConstraint
	result.Violations = violations
	result.BalanceSatisfied = p.BalanceSatisfied(plan)
	result.Unfilled = len(plan.Unfilled())
	return plan, result
}

// invigModel adapts the invigilation plan to the generic optimize.Model: the state is
// the *Plan, Cost is the registry cost, a Propose is one proposeMove kept hard-feasible
// (apply → registry veto → undo). Converged() reports the balance/coverage target and
// caches it for the progress bridge (the engine calls it when best is snapshotted).
type invigModel struct {
	p            *Problem
	reg          *Registry
	plan         *Plan
	movable      []int
	bestBalance  bool
	bestUnfilled int
}

func (m *invigModel) Cost() float64 {
	c, _, _ := m.reg.Cost(m.p, m.plan)
	return c
}
func (m *invigModel) Snapshot() any { return m.plan.Clone() }
func (m *invigModel) Restore(s any) { m.plan = s.(*Plan) }
func (m *invigModel) Propose(rng *rand.Rand) func() {
	changes := proposeMove(m.p, m.plan, rng, m.movable)
	if changes == nil {
		return nil
	}
	m.plan.apply(changes)
	if !feasible(m.p, m.reg, m.plan, changes) {
		m.plan.undo(changes)
		return nil
	}
	return func() { m.plan.undo(changes) }
}
func (m *invigModel) Converged() bool {
	m.bestBalance = m.p.BalanceSatisfied(m.plan)
	m.bestUnfilled = len(m.plan.Unfilled())
	return m.bestBalance && m.bestUnfilled == 0
}

// Greedy fills the open positions, most-constrained first, always choosing the
// allowed invigilator currently furthest below their target minutes. Positions
// that no invigilator may take are left open.
func Greedy(p *Problem, reg *Registry, rng *rand.Rand) *Plan {
	plan := NewPlan(p)

	type posElig struct {
		idx  int
		elig int
	}
	movable := make([]posElig, 0, len(p.Positions))
	for i := range p.Positions {
		if _, fixed := p.Fixed[i]; fixed {
			continue
		}
		movable = append(movable, posElig{idx: i, elig: staticEligibleCount(p, i)})
	}
	sort.Slice(movable, func(a, b int) bool {
		if movable[a].elig != movable[b].elig {
			return movable[a].elig < movable[b].elig
		}
		return movable[a].idx < movable[b].idx
	})

	for _, m := range movable {
		best := Unassigned
		bestScore := math.Inf(-1)
		for j := range p.Invigilators {
			v := p.Invigilators[j].ID
			if !reg.Allows(p, plan, m.idx, v) {
				continue
			}
			deficit := float64(p.Invigilators[j].TargetMinutes - plan.DoingMinutes(v))
			score := deficit + rng.Float64()*0.001 // tiny tie-break
			if score > bestScore {
				bestScore = score
				best = v
			}
		}
		if best != Unassigned {
			plan.Set(m.idx, best)
		}
	}
	return plan
}

// proposeMove returns a small, possibly empty, set of reassignments: a single
// reassign (70%), an unassign (10%) or a swap of two positions (20%).
func proposeMove(p *Problem, plan *Plan, rng *rand.Rand, movable []int) []change {
	a := movable[rng.Intn(len(movable))]
	switch r := rng.Float64(); {
	case r < 0.7:
		v := p.Invigilators[rng.Intn(len(p.Invigilators))].ID
		if v == plan.Assign[a] {
			return nil
		}
		return []change{{pos: a, oldInvig: plan.Assign[a], newInvig: v}}
	case r < 0.8:
		if plan.Assign[a] == Unassigned {
			return nil
		}
		return []change{{pos: a, oldInvig: plan.Assign[a], newInvig: Unassigned}}
	default:
		b := movable[rng.Intn(len(movable))]
		if b == a {
			return nil
		}
		va, vb := plan.Assign[a], plan.Assign[b]
		if va == vb {
			return nil
		}
		return []change{
			{pos: a, oldInvig: va, newInvig: vb},
			{pos: b, oldInvig: vb, newInvig: va},
		}
	}
}

func (pl *Plan) apply(changes []change) {
	for _, c := range changes {
		if c.newInvig == Unassigned {
			pl.Clear(c.pos)
		} else {
			pl.Set(c.pos, c.newInvig)
		}
	}
}

func (pl *Plan) undo(changes []change) {
	for i := len(changes) - 1; i >= 0; i-- {
		c := changes[i]
		if c.oldInvig == Unassigned {
			pl.Clear(c.pos)
		} else {
			pl.Set(c.pos, c.oldInvig)
		}
	}
}

// feasible reports whether the just-applied changes keep every touched position
// hard-feasible.
func feasible(p *Problem, reg *Registry, plan *Plan, changes []change) bool {
	for _, c := range changes {
		if c.newInvig != Unassigned && !reg.Allows(p, plan, c.pos, c.newInvig) {
			return false
		}
	}
	return true
}

func movablePositions(p *Problem) []int {
	movable := make([]int, 0, len(p.Positions))
	for i := range p.Positions {
		if _, fixed := p.Fixed[i]; !fixed {
			movable = append(movable, i)
		}
	}
	return movable
}

// staticEligibleCount counts invigilators that could in principle take the
// position (availability and own exam), ignoring the dynamic plan state.
func staticEligibleCount(p *Problem, posIdx int) int {
	pos := p.Positions[posIdx]
	n := 0
	for i := range p.Invigilators {
		in := &p.Invigilators[i]
		if in.Available(pos) && !in.OwnExamSlots[pos.SlotKey()] {
			n++
		}
	}
	return n
}
