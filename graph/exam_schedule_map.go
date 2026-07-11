package graph

import (
	"sort"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams"
)

// examScheduleReport maps the plexams result to the GraphQL ExamScheduleReport.
func examScheduleReport(r *plexams.ExamScheduleResult) *model.ExamScheduleReport {
	costs := make([]*model.ConstraintCost, 0, len(r.CostByConstraint))
	for name, cost := range r.CostByConstraint {
		costs = append(costs, &model.ConstraintCost{Name: name, Cost: cost})
	}
	sort.Slice(costs, func(i, j int) bool { return costs[i].Cost > costs[j].Cost })

	d := r.Diagnostics
	return &model.ExamScheduleReport{
		Units:            r.Units,
		Fixed:            r.Fixed,
		Placed:           r.Placed,
		Unplaced:         r.Unplaced,
		UnplacedAncodes:  r.UnplacedAncodes,
		HardViolations:   r.HardViolations,
		Cost:             r.Cost,
		CostByConstraint: costs,
		Iterations:       r.Iterations,
		Seed:             r.Seed,
		StoppedEarly:     r.StoppedEarly,
		Written:          r.Written,
		Diagnostics: &model.ExamScheduleDiagnostics{
			Students:             d.Students,
			Pairs:                d.Pairs,
			Overlaps:             d.SameSlot,
			TooClose:             d.Adjacent,
			SameDay:              d.SameDay,
			NextDay:              d.NextDay,
			Within3:              d.Within3,
			Further:              d.Further,
			StudentsWithTooClose: d.StudentsWithAdjacent,
			StudentsWithSameDay:  d.StudentsWithSameDay,
			WorstStudentPenalty:  d.WorstStudentPenalty,
			MaxSeatsAt:           d.MaxSlotSeats,
			StarttimesUsed:       d.SlotsUsed,
			SlotsOverThreshold:   d.SlotsOverThreshold,
			MaxExamsAt:           d.MaxExamsPerSlot,
		},
		Conflicts:         r.Conflicts,
		ResolvedConflicts: r.ResolvedConflicts,
		ExahmNtaAncodes:   r.ExahmNtaAncodes,
	}
}
