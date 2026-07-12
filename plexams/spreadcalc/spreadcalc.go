// Package spreadcalc holds the pure student-spread math for the exam-schedule
// quality statistics: classifying the gap between two of a student's consecutive
// exams by the number of exam-free calendar days between them, turning that into a
// Carter-style proximity cost, and bucketing it. All functions are I/O-free; the DB
// access and aggregation stay in the plexams package.
//
// "Free day" counts CALENDAR days: a Friday→Monday gap is two free days (Sat/Sun),
// matching how a student actually experiences rest days.
package spreadcalc

import (
	"math"
	"sort"
	"time"

	"github.com/obcode/plexams.go/plexams/conflictcalc"
)

// PairGap is the outcome of one consecutive-exam pair, expressed as the number of
// exam-free calendar days between the two exams, with two sentinels for the sub-day
// cases (both worse than any gap of >= 0 free days):
//
//	GapOverlap (-2) – the exams overlap or leave less than the required buffer.
//	GapSameDay (-1) – two exams on the same calendar day (incl. "too close").
//	0               – two consecutive calendar days (no free day between).
//	k >= 1          – k exam-free calendar days between the two exams.
type PairGap int

const (
	GapOverlap PairGap = -2
	GapSameDay PairGap = -1
)

// Bucket keys, worst (closest in time) to mildest.
const (
	KeyOverlap   = "OVERLAP"
	KeySameDay   = "SAME_DAY"
	KeyAdjacent  = "ADJACENT"
	KeyOneFree   = "ONE_FREE"
	KeyTwoFree   = "TWO_FREE"
	KeyThreePlus = "THREE_PLUS_FREE"
)

// BucketOrder is the stable worst-first display order of the buckets.
var BucketOrder = []string{KeyOverlap, KeySameDay, KeyAdjacent, KeyOneFree, KeyTwoFree, KeyThreePlus}

// ExamTime is one placed exam of a student: its absolute start and (student-specific,
// NTA-aware) end time.
type ExamTime struct {
	Start time.Time
	End   time.Time
}

// StudentSpread is the computed spread outcome for one student (>= 2 exams).
type StudentSpread struct {
	// Pairs holds the gap of every consecutive-exam pair (len = #exams-1).
	Pairs []PairGap
	// MinGap is the tightest (numerically smallest) pair — the student's worst case.
	MinGap PairGap
	// ProximityCost is the Carter-style penalty summed over the consecutive pairs.
	ProximityCost float64
	// MaxExamsPerDay is the most exams the student has on any single calendar day.
	MaxExamsPerDay int
}

// ClassifyPair classifies two of a student's placed exams by their absolute times.
// It reuses conflictcalc.TimeProximity for the overlap/same-day decision (so the
// examGap buffer stays consistent with the rest of the code) and otherwise counts
// the exam-free calendar days between them.
func ClassifyPair(startA, endA, startB, endB time.Time, examGapMin, notTooCloseMin int) PairGap {
	rank, _ := conflictcalc.TimeProximity(startA, endA, startB, endB, examGapMin, notTooCloseMin)
	switch rank {
	case 4: // Overlap
		return GapOverlap
	case 3, 2: // TooClose / SameDay
		return GapSameDay
	}
	// Different calendar days: free days between = (calendar-day distance) - 1.
	return PairGap(calendarDaysBetween(startA, startB) - 1)
}

// PairCost is the Carter-style proximity penalty of a single gap (lower = better).
// The classic scheme halves the weight per extra free day; same-day and overlap sit
// above the adjacent-day weight. Gaps of >= 4 free days cost nothing.
func PairCost(g PairGap) float64 {
	switch g {
	case GapOverlap:
		return 32
	case GapSameDay:
		return 16
	case 0:
		return 8
	case 1:
		return 4
	case 2:
		return 2
	case 3:
		return 1
	default:
		return 0
	}
}

// BucketKey maps a gap to its distribution bucket.
func BucketKey(g PairGap) string {
	switch g {
	case GapOverlap:
		return KeyOverlap
	case GapSameDay:
		return KeySameDay
	case 0:
		return KeyAdjacent
	case 1:
		return KeyOneFree
	case 2:
		return KeyTwoFree
	default:
		return KeyThreePlus
	}
}

// ComputeStudent computes the spread outcome for one student's placed exams. The
// input need not be sorted. With fewer than two exams it returns the zero value with
// an empty Pairs slice (the caller treats such students as having no gap).
func ComputeStudent(exams []ExamTime, examGapMin, notTooCloseMin int) StudentSpread {
	if len(exams) < 2 {
		return StudentSpread{MaxExamsPerDay: len(exams)}
	}
	sorted := make([]ExamTime, len(exams))
	copy(sorted, exams)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Start.Before(sorted[j].Start) })

	res := StudentSpread{Pairs: make([]PairGap, 0, len(sorted)-1), MinGap: PairGap(math.MaxInt32)}
	for i := 0; i+1 < len(sorted); i++ {
		g := ClassifyPair(sorted[i].Start, sorted[i].End, sorted[i+1].Start, sorted[i+1].End, examGapMin, notTooCloseMin)
		res.Pairs = append(res.Pairs, g)
		res.ProximityCost += PairCost(g)
		if g < res.MinGap {
			res.MinGap = g
		}
	}
	res.MaxExamsPerDay = maxExamsPerDay(sorted)
	return res
}

// maxExamsPerDay returns the most exams falling on a single calendar day.
func maxExamsPerDay(exams []ExamTime) int {
	perDay := make(map[[3]int]int, len(exams))
	best := 0
	for _, e := range exams {
		y, m, d := e.Start.Date()
		key := [3]int{y, int(m), d}
		perDay[key]++
		if perDay[key] > best {
			best = perDay[key]
		}
	}
	return best
}

func calendarDaysBetween(a, b time.Time) int {
	da := time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, a.Location())
	db := time.Date(b.Year(), b.Month(), b.Day(), 0, 0, 0, 0, b.Location())
	diff := db.Sub(da)
	if diff < 0 {
		diff = -diff
	}
	return int(math.Round(diff.Hours() / 24))
}
