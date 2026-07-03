// Package invigcalc computes the fair distribution of invigilation duties: given the total
// invigilation minutes to cover and each invigilator's factor and already-credited
// contributions, it derives the per-invigilator target minutes (factor-weighted, with
// largest-remainder rounding so the targets sum exactly to the work) and the per-invigilator
// todo summary. These are pure functions over graph/model types with no I/O, split out of
// the plexams package so the correctness-critical math is isolated and unit-tested.
package invigcalc

import (
	"math"
	"sort"

	set "github.com/deckarep/golang-set/v2"
	"github.com/obcode/plexams.go/graph/model"
)

// FairTargets computes the factor-weighted invigilation minutes each invigilator should be
// planned for, given workMinutes (= SumExamRooms + SumReserve) of actual invigilation that
// must be covered and the already credited other contributions (Beisitz, Live-Coding,
// Master, ...) of every invigilator.
//
// It solves
//
//	Σ_active max(0, T·Factor_i − contributions_i) = workMinutes
//
// as a fixed point: an invigilator whose contributions already reach or exceed their share
// (T·Factor_i) does no invigilation, so they drop out of *both* numerator and denominator.
// Their over-contribution is "Schicksal" -- it cannot be redistributed because only the
// workMinutes that actually exist can be shared. Removing such an invigilator only raises T
// (and thus the threshold), so the active set shrinks monotonically and the loop converges
// in at most len(reqs) rounds.
//
// It returns:
//   - todoPerInvigilator: the fair T, rounded to the nearest integer (display value only).
//   - countedContributions: the contributions of the still-active invigilators,
//   - targets: the integer invigilation minutes each invigilator (by teacher ID) should be
//     planned for, net of their other contributions. The float shares T·Factor_i −
//     contributions_i sum to exactly workMinutes; they are rounded to integers with the
//     largest-remainder method so the *sum of targets stays exactly workMinutes*.
//   - enough: invigilators who already contributed at least their fair share and therefore
//     do no invigilation (target 0).
func FairTargets(workMinutes int, reqs []*model.Invigilator) (
	todoPerInvigilator, countedContributions int,
	targets map[int]int, enough map[int]bool,
) {
	targets = make(map[int]int, len(reqs))
	enough = make(map[int]bool, len(reqs))
	markEnough := func(invigilator *model.Invigilator) {
		if invigilator.Teacher != nil {
			enough[invigilator.Teacher.ID] = true
		}
	}

	active := make([]*model.Invigilator, 0, len(reqs))
	for _, invigilator := range reqs {
		if invigilator.Requirements != nil && invigilator.Requirements.Factor > 0 {
			active = append(active, invigilator)
		} else {
			markEnough(invigilator) // free semester / not working: no invigilation
		}
	}

	t := 0.0
	for {
		sumFactor := 0.0
		sumContributions := 0
		for _, invigilator := range active {
			sumFactor += invigilator.Requirements.Factor
			sumContributions += invigilator.Requirements.AllContributions
		}
		if sumFactor == 0 {
			for _, invigilator := range active {
				markEnough(invigilator)
			}
			return 0, 0, targets, enough
		}

		t = (float64(workMinutes) + float64(sumContributions)) / sumFactor

		stillActive := make([]*model.Invigilator, 0, len(active))
		for _, invigilator := range active {
			if float64(invigilator.Requirements.AllContributions) < t*invigilator.Requirements.Factor {
				stillActive = append(stillActive, invigilator)
			} else {
				markEnough(invigilator) // over-contributed: drops out, target 0
			}
		}

		if len(stillActive) == len(active) {
			countedContributions = sumContributions
			break
		}
		active = stillActive
	}

	// Largest-remainder (Hamilton) rounding: floor every share, then hand the missing
	// minutes to the largest fractional parts. Σ_active share_i == workMinutes exactly (the
	// fixed point), so the deficit is in [0, len(active)) and the rounded targets sum to
	// exactly workMinutes.
	type share struct {
		id    int
		floor int
		frac  float64
	}
	shares := make([]share, 0, len(active))
	sumFloor := 0
	for _, invigilator := range active {
		raw := t*invigilator.Requirements.Factor - float64(invigilator.Requirements.AllContributions)
		fl := int(math.Floor(raw))
		shares = append(shares, share{id: invigilator.Teacher.ID, floor: fl, frac: raw - float64(fl)})
		sumFloor += fl
	}

	sort.Slice(shares, func(i, j int) bool { return shares[i].frac > shares[j].frac })
	deficit := workMinutes - sumFloor
	for i, s := range shares {
		target := s.floor
		if i < deficit {
			target++
		}
		targets[s.id] = target
	}

	return int(math.Round(t)), countedContributions, targets, enough
}

// Todos builds the per-invigilator todos: the credited doingMinutes (self invigilations
// count 0, reserves a fixed 60 min, see SumReserve), the set of invigilation days and the
// given fair target.
func Todos(invigilations []*model.Invigilation, totalMinutes int, enough bool) *model.InvigilatorTodos {
	invigilationSet := set.NewSet[int]()
	doingMinutes := 0
	for _, invigilation := range invigilations {
		invigilationSet.Add(invigilation.Slot.DayNumber)
		if !invigilation.IsSelfInvigilation {
			if invigilation.IsReserve {
				// reserves are credited with a fixed 60 min (matches SumReserve), not the
				// slot's actual time block stored in Duration.
				doingMinutes += 60
			} else {
				doingMinutes += invigilation.Duration
			}
		}
	}
	invigilationDays := invigilationSet.ToSlice()
	sort.Ints(invigilationDays)

	return &model.InvigilatorTodos{
		TotalMinutes:     totalMinutes,
		DoingMinutes:     doingMinutes,
		Enough:           enough,
		InvigilationDays: invigilationDays,
		Invigilations:    invigilations,
	}
}
