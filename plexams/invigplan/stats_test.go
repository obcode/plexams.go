package invigplan

import (
	"reflect"
	"testing"
)

func TestDistributionSortedCountsAndString(t *testing.T) {
	d := Distribution{Kind: KindReserve, ByCount: map[int]int{2: 17, 0: 4, 1: 48}, Total: 82, Max: 2}
	if got := d.SortedCounts(); !reflect.DeepEqual(got, []int{0, 1, 2}) {
		t.Errorf("SortedCounts = %v, want [0 1 2]", got)
	}
	if got := d.String(); got != "0:4  1:48  2:17" {
		t.Errorf("String = %q, want %q", got, "0:4  1:48  2:17")
	}
	// empty distribution
	if got := (Distribution{ByCount: map[int]int{}}).String(); got != "" {
		t.Errorf("empty String = %q, want empty", got)
	}
}

func TestResultSortedCosts(t *testing.T) {
	r := Result{CostByConstraint: map[string]float64{
		"minuteBalance": 1200,
		"coverage":      0, // filtered out (not > 0)
		"timeGap":       300,
		"maxDays":       800,
	}}
	got := r.SortedCosts()
	want := []CostItem{
		{Name: "minuteBalance", Cost: 1200},
		{Name: "maxDays", Cost: 800},
		{Name: "timeGap", Cost: 300},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SortedCosts = %+v, want %+v", got, want)
	}
}

// deviationProblem builds a problem with six 90-minute positions and four invigilators
// whose targets let us drive precise deviations from a plan.
func deviationProblem() *Problem {
	return &Problem{
		Positions: []Position{
			{Minutes: 90}, {Minutes: 90}, {Minutes: 90},
			{Minutes: 90}, {Minutes: 90}, {Minutes: 90},
		},
		Invigilators: []Invigilator{
			{ID: 1, TargetMinutes: 300}, // will do 0   -> dev -300, rel 1.0
			{ID: 2, TargetMinutes: 300}, // will do 360 -> dev +60,  rel 0.2
			{ID: 3, TargetMinutes: 60},  // will do 180 -> dev +120, rel 2.0
			{ID: 4, TargetMinutes: 0},   // will do 0   -> dev 0, excluded
		},
		Fixed:        map[int]int{},
		ToleranceMin: 60,
	}
}

func TestDeviationOutliers(t *testing.T) {
	p := deviationProblem()
	plan := NewPlan(p)
	plan.Set(0, 2)
	plan.Set(1, 2)
	plan.Set(2, 2)
	plan.Set(3, 2) // invig 2: 360 min
	plan.Set(4, 3)
	plan.Set(5, 3) // invig 3: 180 min

	got := p.DeviationOutliers(plan, 0) // all
	want := []Outlier{
		{InvigilatorID: 3, Doing: 180, Target: 60, Open: -120, Percent: -200},
		{InvigilatorID: 1, Doing: 0, Target: 300, Open: 300, Percent: 100},
		{InvigilatorID: 2, Doing: 360, Target: 300, Open: -60, Percent: -20},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d outliers, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("outlier[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestDeviationOutliersTopN(t *testing.T) {
	p := deviationProblem()
	plan := NewPlan(p)
	plan.Set(0, 2)
	plan.Set(1, 2)
	plan.Set(2, 2)
	plan.Set(3, 2)
	plan.Set(4, 3)
	plan.Set(5, 3)

	got := p.DeviationOutliers(plan, 2)
	if len(got) != 2 || got[0].InvigilatorID != 3 || got[1].InvigilatorID != 1 {
		t.Errorf("topN=2 = %+v, want invigs [3 1] by relative deviation", got)
	}
}

func TestDeviationOutliersNone(t *testing.T) {
	p := deviationProblem()
	plan := NewPlan(p) // nobody assigned; invigs 1/2/3 all under target, 4 on target
	got := p.DeviationOutliers(plan, 0)
	// invig 4 (target 0, doing 0) is excluded; the others deviate
	for _, o := range got {
		if o.InvigilatorID == 4 {
			t.Errorf("invigilator on target (0/0) should be excluded, got %+v", o)
		}
	}
}
