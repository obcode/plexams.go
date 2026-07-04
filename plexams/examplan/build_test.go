package examplan

import (
	"reflect"
	"testing"
)

func TestAttractPairs(t *testing.T) {
	// units 0,1: same module+program, different examer -> parallel sections -> attract
	// unit 2: same module+program as 0/1 but SAME examer as 0 -> no parallel-section pair
	// units 3,4: small exams (Seats<=5) of the same examer 99 -> attract
	// unit 5: fixed -> ignored entirely
	units := []Unit{
		{Module: "M1", Program: "IF", Examer: 10, Seats: 100},              // 0
		{Module: "M1", Program: "IF", Examer: 20, Seats: 100},              // 1
		{Module: "M1", Program: "IF", Examer: 10, Seats: 100},              // 2
		{Module: "M2", Program: "IB", Examer: 99, Seats: 3},                // 3
		{Module: "M3", Program: "IB", Examer: 99, Seats: 4},                // 4
		{Module: "M1", Program: "IF", Examer: 30, Seats: 100, Fixed: true}, // 5 (ignored)
	}
	got := AttractPairs(units, 5)

	want := []AttractPair{
		{A: 0, B: 1, Weight: 1}, // parallel sections (examer 10 vs 20)
		{A: 1, B: 2, Weight: 1}, // parallel sections (examer 20 vs 10)
		{A: 3, B: 4, Weight: 1}, // small exams, same examer 99
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("AttractPairs =\n  %+v\nwant\n  %+v", got, want)
	}
}

func TestAttractPairsSmallExamExcludesUnknownExamerAndLarge(t *testing.T) {
	units := []Unit{
		{Module: "A", Program: "P", Examer: 0, Seats: 2}, // 0: examer 0 (unknown) -> not clustered
		{Module: "B", Program: "P", Examer: 0, Seats: 2}, // 1: examer 0 (unknown)
		{Module: "C", Program: "P", Examer: 7, Seats: 6}, // 2: too large (>5) for same-examer
		{Module: "D", Program: "P", Examer: 7, Seats: 6}, // 3: too large
	}
	if got := AttractPairs(units, 5); len(got) != 0 {
		t.Errorf("AttractPairs = %+v, want none", got)
	}
}

func TestIntersectAllowed(t *testing.T) {
	tests := []struct {
		name string
		sets [][]int
		want []int
	}{
		{"all empty -> nil (all allowed)", [][]int{{}, {}}, nil},
		{"no sets -> nil", nil, nil},
		{"empty is skipped", [][]int{{}, {1, 2, 3}}, []int{1, 2, 3}},
		{"intersection", [][]int{{1, 2, 3}, {2, 3, 4}}, []int{2, 3}},
		{"three-way, sorted", [][]int{{5, 1, 2, 3}, {1, 2, 3}, {2, 3, 9}}, []int{2, 3}},
		{"disjoint -> empty", [][]int{{1, 2}, {3, 4}}, []int{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IntersectAllowed(tt.sets)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("IntersectAllowed(%v) = %v, want %v", tt.sets, got, tt.want)
			}
		})
	}
}

func TestIntersectSlots(t *testing.T) {
	tests := []struct {
		name string
		a, b []int
		want []int
	}{
		{"empty a -> copy of b", nil, []int{1, 2, 3}, []int{1, 2, 3}},
		{"intersection preserves a's order", []int{3, 1, 2}, []int{1, 2}, []int{1, 2}},
		{"no overlap", []int{1, 2}, []int{3, 4}, []int{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IntersectSlots(tt.a, tt.b)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("IntersectSlots(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
