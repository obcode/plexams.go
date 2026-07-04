// Package conflictcalc holds the pure schedule-conflict math: normalising an ancode
// pair into a stable key, ranking a conflict's proximity (how close in time two of a
// student's exams ended up), the proximity of two placed slots, and diffing a freshly
// generated conflict list against the saved one. All functions are I/O-free over
// graph/model types; the DB access and aggregation stay in the plexams package.
package conflictcalc

import (
	"math"

	"github.com/obcode/plexams.go/graph/model"
)

// Proximity labels, worst (closest in time) to mildest. Two exams closer than NextDay
// count as a conflict; NextDay and farther are acceptable.
const (
	SameSlot = "SAME_SLOT"
	Adjacent = "ADJACENT"
	SameDay  = "SAME_DAY"
	NextDay  = "NEXT_DAY"
)

// NormPair returns the two ancodes in ascending order. Ratings, decisions and
// canShareSlot are stored order-independently, so this is the canonical pair key.
func NormPair(a, b int) (int, int) {
	if a > b {
		return b, a
	}
	return a, b
}

// ProximityRank orders a proximity label from worst (SAME_SLOT) to mildest, so two
// plans' conflicts can be compared ("got worse / better") and a conflict list sorted
// worst-first. Unknown/empty labels rank 0.
func ProximityRank(label string) int {
	switch label {
	case SameSlot:
		return 4
	case Adjacent:
		return 3
	case SameDay:
		return 2
	case NextDay:
		return 1
	}
	return 0
}

// SlotProximity returns the proximity rank and label of two placed slots (higher rank =
// closer in time = worse); rank 0 with an empty label means far enough apart to not
// count as a conflict.
func SlotProximity(a, b *model.Slot) (int, string) {
	if a.DayNumber == b.DayNumber {
		switch absInt(a.SlotNumber - b.SlotNumber) {
		case 0:
			return 4, SameSlot
		case 1:
			return 3, Adjacent
		default:
			return 2, SameDay
		}
	}
	diff := a.Starttime.Sub(b.Starttime)
	if diff < 0 {
		diff = -diff
	}
	if int(math.Round(diff.Hours()/24)) == 1 {
		return 1, NextDay
	}
	return 0, ""
}

// DiffAgainstSaved tags each generated conflict with its status relative to the
// saved-plan conflicts — "new", "worse", "better" or "unchanged" — and returns the
// resolved ones (present in the saved plan, gone in the generated one), tagged
// "resolved". Both lists are keyed on the ancode pair (Ancode1 < Ancode2). The
// generated conflicts are mutated in place (DiffStatus set).
func DiffAgainstSaved(generated, saved []*model.ExamScheduleConflict) []*model.ExamScheduleConflict {
	savedByPair := make(map[[2]int]*model.ExamScheduleConflict, len(saved))
	for _, c := range saved {
		savedByPair[[2]int{c.Ancode1, c.Ancode2}] = c
	}
	seen := make(map[[2]int]bool, len(generated))
	for _, c := range generated {
		key := [2]int{c.Ancode1, c.Ancode2}
		seen[key] = true
		old, ok := savedByPair[key]
		switch {
		case !ok:
			c.DiffStatus = "new"
		case ProximityRank(c.Proximity) > ProximityRank(old.Proximity):
			c.DiffStatus = "worse"
		case ProximityRank(c.Proximity) < ProximityRank(old.Proximity):
			c.DiffStatus = "better"
		default:
			c.DiffStatus = "unchanged"
		}
	}
	resolved := make([]*model.ExamScheduleConflict, 0)
	for _, c := range saved {
		if !seen[[2]int{c.Ancode1, c.Ancode2}] {
			c.DiffStatus = "resolved"
			resolved = append(resolved, c)
		}
	}
	return resolved
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
