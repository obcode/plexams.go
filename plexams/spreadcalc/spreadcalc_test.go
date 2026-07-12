package spreadcalc

import (
	"testing"
	"time"
)

const (
	testGap      = 30 // examGapMinutes
	testNotClose = 90 // notTooCloseMinutes
)

// at builds a time on the given calendar day (2024-01-dd) at hh:mm local.
func at(day, hh, mm int) time.Time {
	return time.Date(2024, 1, day, hh, mm, 0, 0, time.Local)
}

func TestClassifyPair(t *testing.T) {
	tests := []struct {
		name           string
		s1, e1, s2, e2 time.Time
		want           PairGap
	}{
		{"overlap same day (< gap)", at(5, 10, 0), at(5, 11, 0), at(5, 11, 15), at(5, 12, 15), GapOverlap},
		{"same day far apart", at(5, 8, 0), at(5, 9, 30), at(5, 14, 0), at(5, 15, 30), GapSameDay},
		{"too close same day", at(5, 10, 0), at(5, 11, 0), at(5, 11, 40), at(5, 12, 40), GapSameDay},
		{"adjacent days", at(5, 10, 0), at(5, 11, 0), at(6, 10, 0), at(6, 11, 0), 0},
		{"one free day (Mon->Wed)", at(8, 10, 0), at(8, 11, 0), at(10, 10, 0), at(10, 11, 0), 1},
		{"weekend two free days (Fri->Mon)", at(5, 10, 0), at(5, 11, 0), at(8, 10, 0), at(8, 11, 0), 2},
		{"order independent", at(10, 10, 0), at(10, 11, 0), at(8, 10, 0), at(8, 11, 0), 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyPair(tt.s1, tt.e1, tt.s2, tt.e2, testGap, testNotClose); got != tt.want {
				t.Errorf("ClassifyPair = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestComputeStudent(t *testing.T) {
	// exams: day5 08:00-09:00, day5 14:00-15:00 (same day), day8 10:00-11:00 (Fri->Mon = 2 free)
	exams := []ExamTime{
		{Start: at(5, 8, 0), End: at(5, 9, 0)},
		{Start: at(8, 10, 0), End: at(8, 11, 0)},
		{Start: at(5, 14, 0), End: at(5, 15, 0)},
	}
	sp := ComputeStudent(exams, testGap, testNotClose)

	if len(sp.Pairs) != 2 {
		t.Fatalf("Pairs len = %d, want 2", len(sp.Pairs))
	}
	// sorted: (day5 08, day5 14) = same day; (day5 14, day8 10) = 2 free days
	if sp.Pairs[0] != GapSameDay || sp.Pairs[1] != 2 {
		t.Errorf("Pairs = %v, want [SameDay, 2]", sp.Pairs)
	}
	if sp.MinGap != GapSameDay {
		t.Errorf("MinGap = %d, want %d (same day is the worst)", sp.MinGap, GapSameDay)
	}
	if sp.MaxExamsPerDay != 2 {
		t.Errorf("MaxExamsPerDay = %d, want 2", sp.MaxExamsPerDay)
	}
	// proximity: same day (16) + 2 free days (2) = 18
	if sp.ProximityCost != 18 {
		t.Errorf("ProximityCost = %v, want 18", sp.ProximityCost)
	}
}

func TestComputeStudentSingleExam(t *testing.T) {
	sp := ComputeStudent([]ExamTime{{Start: at(5, 8, 0), End: at(5, 9, 0)}}, testGap, testNotClose)
	if len(sp.Pairs) != 0 {
		t.Errorf("single exam should have no pairs, got %d", len(sp.Pairs))
	}
	if sp.MaxExamsPerDay != 1 {
		t.Errorf("MaxExamsPerDay = %d, want 1", sp.MaxExamsPerDay)
	}
}

func TestBucketKey(t *testing.T) {
	cases := map[PairGap]string{
		GapOverlap: KeyOverlap,
		GapSameDay: KeySameDay,
		0:          KeyAdjacent,
		1:          KeyOneFree,
		2:          KeyTwoFree,
		3:          KeyThreePlus,
		7:          KeyThreePlus,
	}
	for g, want := range cases {
		if got := BucketKey(g); got != want {
			t.Errorf("BucketKey(%d) = %s, want %s", g, got, want)
		}
	}
}
