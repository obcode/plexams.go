// Package conflictcalc holds the pure schedule-conflict math: normalising an ancode
// pair into a stable key, ranking a conflict's proximity (how close in time two of a
// student's exams ended up), classifying two placed exams by their absolute times and
// durations, and diffing a freshly generated conflict list against the saved one. All
// functions are I/O-free over graph/model types; the DB access and aggregation stay in
// the plexams package.
package conflictcalc

import (
	"math"
	"time"

	"github.com/obcode/plexams.go/graph/model"
)

// Proximity labels, worst (closest in time) to mildest. Two exams closer than NextDay
// count as a conflict; NextDay and farther are acceptable.
//
//	Overlap  – the exams (incl. NTA time) run at the same time, or leave a student less
//	           than the travel/break buffer (examGapMinutes) between them: impossible.
//	TooClose – same day, start times closer than notTooCloseMinutes: undesirable.
//	SameDay  – same day, but far enough apart.
//	NextDay  – consecutive calendar day.
const (
	Overlap  = "OVERLAP"
	TooClose = "TOO_CLOSE"
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

// ProximityRank orders a proximity label from worst (OVERLAP) to mildest, so two plans'
// conflicts can be compared ("got worse / better") and a conflict list sorted
// worst-first. Unknown/empty labels rank 0.
func ProximityRank(label string) int {
	switch label {
	case Overlap:
		return 4
	case TooClose:
		return 3
	case SameDay:
		return 2
	case NextDay:
		return 1
	}
	return 0
}

// TimeProximity classifies two of a student's placed exams by their absolute start/end
// times (end = start + duration incl. NTA), returning a rank (higher = worse) and label.
// examGapMinutes is the required travel/break buffer between two exams; a smaller gap
// (including a real overlap) is an OVERLAP. notTooCloseMinutes is the start-to-start
// distance below which two same-day exams are flagged TOO_CLOSE. Rank 0 / "" means far
// enough apart to not count as a conflict.
func TimeProximity(startA, endA, startB, endB time.Time, examGapMinutes, notTooCloseMinutes int) (int, string) {
	// identify the earlier and later exam by start time
	earlierStart, earlierEnd, laterStart := startA, endA, startB
	if startB.Before(startA) {
		earlierStart, earlierEnd, laterStart = startB, endB, startA
	}

	gap := laterStart.Sub(earlierEnd) // break between the earlier exam's end and the later's start
	if gap < time.Duration(examGapMinutes)*time.Minute {
		return 4, Overlap
	}
	if sameCalendarDay(earlierStart, laterStart) {
		if laterStart.Sub(earlierStart) < time.Duration(notTooCloseMinutes)*time.Minute {
			return 3, TooClose
		}
		return 2, SameDay
	}
	if daysApart(earlierStart, laterStart) == 1 {
		return 1, NextDay
	}
	return 0, ""
}

func sameCalendarDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func daysApart(a, b time.Time) int {
	diff := b.Sub(a)
	if diff < 0 {
		diff = -diff
	}
	return int(math.Round(diff.Hours() / 24))
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
