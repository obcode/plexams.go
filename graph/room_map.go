package graph

import (
	"sort"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams"
)

// roomPlanReport maps the plexams room-plan result to the GraphQL RoomPlanReport.
func roomPlanReport(r *plexams.RoomPlanResult) *model.RoomPlanReport {
	costs := make([]*model.ConstraintCost, 0, len(r.CostByConstraint))
	for name, cost := range r.CostByConstraint {
		costs = append(costs, &model.ConstraintCost{Name: name, Cost: cost})
	}
	sort.Slice(costs, func(i, j int) bool { return costs[i].Cost > costs[j].Cost })

	return &model.RoomPlanReport{
		Exams:            r.Exams,
		PlacedSeats:      r.PlacedSeats,
		UnplacedSeats:    r.UnplacedSeats,
		Rooms:            r.Rooms,
		HardViolations:   r.HardViolations,
		Cost:             r.Cost,
		CostByConstraint: costs,
		Iterations:       r.Iterations,
		Seed:             r.Seed,
		StoppedEarly:     r.StoppedEarly,
		Written:          r.Written,
		UnplacedExams:    r.UnplacedExams,
	}
}
