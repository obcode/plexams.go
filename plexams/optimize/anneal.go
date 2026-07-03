// Package optimize is a generic simulated-annealing engine shared by the schedule
// generators (exam schedule, pre-plan, invigilations). The engine (Anneal) is
// problem-agnostic: it drives a Model through a geometric cooling schedule with
// Metropolis acceptance and remembers the best state seen. Each generator supplies
//
//   - a Model: its state plus hard-feasible moves (Propose/undo) and the soft cost;
//   - a Registry of self-describing hard/soft Constraints, used both for reporting
//     (per-constraint cost, violations) and for the read-only "which constraints are
//     applied" view in the GUI.
//
// The engine does not know about slots, exams or invigilators; the constraint
// evaluation lives in the Model/Registry the generator wires up.
package optimize

import (
	"math"
	"math/rand"
)

// Options controls the annealing run.
type Options struct {
	Iterations int
	StartTemp  float64
	EndTemp    float64
	Seed       int64

	// StopWhenConverged ends the search early once the Model reports Converged()
	// AND the best state has not improved for StagnationLimit iterations AND the
	// temperature is near its floor (so "no improvement" is meaningful, not just
	// high-temperature wandering). Only applies when the Model implements Converger.
	StopWhenConverged bool
	StagnationLimit   int

	// StrictImprove turns the search into a greedy local search: only strictly cost-
	// reducing moves are accepted (no uphill, no equal-cost lateral moves). Used for a
	// warm start ("improve the current assignment") so the result stays as close to the
	// starting point as possible — nothing moves without lowering the cost. Temperature
	// is then irrelevant, and early stop triggers on stagnation alone.
	StrictImprove bool

	// OnProgress, if set, is called every ProgressEvery iterations with a snapshot
	// of the current best. It is throttled on purpose: per-iteration calls would
	// dominate the runtime with I/O.
	OnProgress    func(Progress)
	ProgressEvery int
}

// DefaultOptions returns sensible defaults. Temperatures are scaled to the typical
// lower-tier soft-cost delta of a single move; a dominant near-hard penalty should be
// weighted large enough to be treated as effectively mandatory.
func DefaultOptions() Options {
	return Options{
		Iterations:        1_000_000,
		StartTemp:         20_000,
		EndTemp:           0.5,
		Seed:              1,
		StopWhenConverged: true,
		StagnationLimit:   30_000,
	}
}

// Progress is a throttled snapshot of the best state for UI feedback. Detail is a
// generator-supplied human-readable status (via the optional Detailer interface).
type Progress struct {
	Iteration int
	Total     int
	BestCost  float64
	Detail    string
}

// Result reports what the run achieved. Per-constraint cost and violations are
// obtained separately from the generator's Registry on the final state.
type Result struct {
	Cost         float64
	Iterations   int
	StoppedEarly bool
}

// Model is the problem-specific state the engine optimizes. Every state the engine
// visits must stay hard-feasible: Propose applies a random hard-feasible move in
// place and returns an undo closure (or nil to skip this step). Cost is the soft
// objective (lower is better). Snapshot/Restore capture and restore the full state
// so the engine can keep the best state seen.
type Model interface {
	Propose(rng *rand.Rand) (undo func())
	Cost() float64
	Snapshot() any
	Restore(any)
}

// Converger is an optional Model capability enabling early stopping: Converged
// reports whether the current state meets the generator's "good enough" criterion
// (e.g. everything placed and all near-hard goals met).
type Converger interface {
	Converged() bool
}

// Detailer is an optional Model capability supplying a human-readable status line
// for progress reporting (evaluated on the best state).
type Detailer interface {
	Detail() string
}

// Anneal improves the Model's current state with simulated annealing and leaves the
// Model restored to the best state found. The Model is responsible for starting from
// (and only ever moving through) hard-feasible states.
func Anneal(m Model, opts Options) Result {
	rng := rand.New(rand.NewSource(opts.Seed)) //nolint:gosec // deterministic, not security relevant
	conv, hasConv := m.(Converger)
	det, hasDet := m.(Detailer)

	cost := m.Cost()
	best := m.Snapshot()
	bestCost := cost
	bestIter := 0
	bestConverged := false
	if hasConv {
		bestConverged = conv.Converged()
	}
	bestDetail := ""
	if hasDet {
		bestDetail = det.Detail()
	}

	progress := opts.OnProgress != nil && opts.ProgressEvery > 0
	result := Result{Iterations: opts.Iterations}

	for it := 0; it < opts.Iterations; it++ {
		if progress && it%opts.ProgressEvery == 0 {
			opts.OnProgress(Progress{Iteration: it, Total: opts.Iterations, BestCost: bestCost, Detail: bestDetail})
		}
		if opts.StopWhenConverged && hasConv && it-bestIter > opts.StagnationLimit &&
			(opts.StrictImprove || temperature(opts, it) <= opts.EndTemp*4) && bestConverged {
			result.Iterations = it
			result.StoppedEarly = true
			break
		}

		undo := m.Propose(rng)
		if undo == nil {
			continue
		}
		newCost := m.Cost()
		delta := newCost - cost
		accept := delta < 0 // strict improvement is always accepted
		if !opts.StrictImprove {
			accept = delta <= 0 || rng.Float64() < math.Exp(-delta/temperature(opts, it))
		}
		if accept {
			cost = newCost
			if cost < bestCost {
				bestCost = cost
				best = m.Snapshot()
				bestIter = it
				if hasConv {
					bestConverged = conv.Converged()
				}
				if hasDet {
					bestDetail = det.Detail()
				}
			}
		} else {
			undo()
		}
	}

	m.Restore(best)
	result.Cost = bestCost
	return result
}

// temperature is a geometric cooling schedule from StartTemp to EndTemp.
func temperature(opts Options, it int) float64 {
	if opts.Iterations <= 1 {
		return opts.EndTemp
	}
	frac := float64(it) / float64(opts.Iterations-1)
	return opts.StartTemp * math.Pow(opts.EndTemp/opts.StartTemp, frac)
}
