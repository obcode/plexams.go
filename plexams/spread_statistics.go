package plexams

import (
	"context"
	"sort"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/spreadcalc"
)

// worstStudentsLimit caps the drill-down list of the most tightly-scheduled students.
const worstStudentsLimit = 25

// lowSampleThreshold is the number of multi-exam students below which a program's
// shares rest on too small a base to be read as reliable percentages.
const lowSampleThreshold = 5

// spreadBucketLabels are the German presentation labels for the distribution buckets.
var spreadBucketLabels = map[string]string{
	spreadcalc.KeyOverlap:   "Überschneidung (Konflikt)",
	spreadcalc.KeySameDay:   "Zwei Prüfungen am selben Tag",
	spreadcalc.KeyAdjacent:  "Aufeinanderfolgende Tage (kein freier Tag)",
	spreadcalc.KeyOneFree:   "1 freier Tag dazwischen",
	spreadcalc.KeyTwoFree:   "2 freie Tage dazwischen",
	spreadcalc.KeyThreePlus: "3+ freie Tage dazwischen",
}

// ExamSpreadStatistics computes the student-centric spread quality of the current
// plan: per student it takes the placed exams (NTA-aware end times, absolute times),
// classifies the gaps between consecutive exams by exam-free calendar days, and
// aggregates that into shares, a distribution histogram, a per-program breakdown, a
// Carter-style proximity index and a worst-off drill-down list.
func (p *Plexams) ExamSpreadStatistics(ctx context.Context) (*model.ExamSpreadStatistics, error) {
	students, err := p.StudentRegsPerStudentPlanned(ctx)
	if err != nil {
		return nil, err
	}
	planEntries, err := p.PlanEntries(ctx)
	if err != nil {
		return nil, err
	}

	examGap := p.semesterConfig.ExamGapMinutes
	notTooClose := p.semesterConfig.NotTooCloseMinutes
	durByAncode := p.examDurationsByAncode(ctx)
	info := p.examInfoMap(ctx)

	// The statistic covers OUR students — those enrolled in an FK07 or MUC.DAI program —
	// because only for them do we hold the COMPLETE set of exams in the period (incl. the
	// external / not-planned-by-me ones). Other-faculty students sit isolated exams with
	// us, so their spread among just-our-exams would be misleadingly optimistic.
	ownProgram := make(map[string]bool)
	for _, prog := range p.zpa.fk07programs {
		ownProgram[prog] = true
	}
	for _, prog := range p.mucdaiProgramNames(ctx) {
		ownProgram[prog] = true
	}
	restrictProgram := len(ownProgram) > 0 // guard against a misconfigured empty list

	// A student's exam counts if it is planned at an absolute time within the exam
	// period — INCLUDING external / not-planned-by-me exams (their time is set by another
	// faculty but is real for the student). Exams timed outside the period have no slot
	// and are ignored.
	inPeriod := periodFilter(p.semesterConfig.From, p.semesterConfig.Until)
	startByAncode := make(map[int]time.Time, len(planEntries))
	plannedSomewhere := make(map[int]bool, len(planEntries))
	for _, pe := range planEntries {
		if pe.Starttime == nil {
			continue
		}
		plannedSomewhere[pe.Ancode] = true
		if inPeriod(*pe.Starttime) {
			startByAncode[pe.Ancode] = *pe.Starttime
		}
	}

	stat := &model.ExamSpreadStatistics{
		ExamGapMinutes:     examGap,
		NotTooCloseMinutes: notTooClose,
	}

	// bucket -> student count (worst-case per student) and pair count (all pairs)
	studentBucket := make(map[string]int)
	pairBucket := make(map[string]int)
	examCountHist := make(map[int]int) // #exams -> #students
	mins := make([]int, 0, len(students))
	var proximitySum float64

	progs := make(map[string]*spreadProgAgg)

	worst := make([]*model.WorstStudent, 0, len(students))

	for _, s := range students {
		if restrictProgram && !ownProgram[s.Program] {
			continue // not one of our (fully-known) students
		}
		// collect this student's placed exams within the period (incl. external ones)
		times := make([]spreadcalc.ExamTime, 0, len(s.ZpaAncodes))
		examAncodes := make([]int, 0, len(s.ZpaAncodes))
		hasUnplanned := false
		for _, ancode := range s.ZpaAncodes {
			if start, ok := startByAncode[ancode]; ok {
				dur := durByAncode[ancode].forStudent(s.Mtknr)
				times = append(times, spreadcalc.ExamTime{Start: start, End: start.Add(time.Duration(dur) * time.Minute)})
				examAncodes = append(examAncodes, ancode)
				continue
			}
			// not on our grid: an unplaced own exam (coverage caveat), not a placed-
			// elsewhere external one.
			if !plannedSomewhere[ancode] && ancode < externalAncodeBase {
				hasUnplanned = true
			}
		}

		nExams := len(times)
		if nExams == 0 {
			if hasUnplanned {
				stat.StudentsWithUnplannedExams++
			}
			continue
		}

		stat.StudentCount++
		stat.TotalPlannedExams += nExams
		if hasUnplanned {
			stat.StudentsWithUnplannedExams++
		}
		if nExams > stat.MaxExamsPerStudent {
			stat.MaxExamsPerStudent = nExams
		}
		examCountHist[capExamCount(nExams)]++

		pa := progs[s.Program]
		if pa == nil {
			pa = &spreadProgAgg{}
			progs[s.Program] = pa
		}
		pa.students++
		pa.examSum += nExams

		if nExams < 2 {
			continue // single exam: no gap to rate
		}

		sp := spreadcalc.ComputeStudent(times, examGap, notTooClose)
		stat.MultiExamStudentCount++
		proximitySum += sp.ProximityCost
		mins = append(mins, int(sp.MinGap))
		if sp.MaxExamsPerDay >= 3 {
			stat.ThreeExamsOneDayCount++
		}

		worstKey := spreadcalc.BucketKey(sp.MinGap)
		studentBucket[worstKey]++
		for _, g := range sp.Pairs {
			pairBucket[spreadcalc.BucketKey(g)]++
		}

		pa.multi++
		pa.minSum += int(sp.MinGap)
		if sp.MinGap >= 1 {
			pa.freeDay++
		}
		if worstKey == spreadcalc.KeySameDay {
			pa.sameDay++
		}

		worst = append(worst, buildWorstStudent(s, sp, times, examAncodes, info))
	}

	// headline shares are a clean partition of the multi-exam students by their worst gap
	denom := float64(stat.MultiExamStudentCount)
	stat.ConflictShare = share(studentBucket[spreadcalc.KeyOverlap], denom)
	stat.SameDayShare = share(studentBucket[spreadcalc.KeySameDay], denom)
	stat.AdjacentDayShare = share(studentBucket[spreadcalc.KeyAdjacent], denom)
	stat.FreeDayShare = share(studentBucket[spreadcalc.KeyOneFree]+studentBucket[spreadcalc.KeyTwoFree]+studentBucket[spreadcalc.KeyThreePlus], denom)
	if stat.StudentCount > 0 {
		stat.AvgExamsPerStudent = float64(stat.TotalPlannedExams) / float64(stat.StudentCount)
	}
	if stat.MultiExamStudentCount > 0 {
		stat.AvgProximityCost = proximitySum / denom
		stat.AvgMinFreeDays = meanInts(mins)
		stat.MedianMinFreeDays = medianInts(mins)
	}

	stat.StudentBuckets = buildBuckets(studentBucket, stat.MultiExamStudentCount)
	pairTotal := 0
	for _, c := range pairBucket {
		pairTotal += c
	}
	stat.PairBuckets = buildBuckets(pairBucket, pairTotal)
	stat.ExamCountBuckets = buildExamCountBuckets(examCountHist, stat.StudentCount)
	stat.ByProgram = buildProgramSpread(progs)

	sort.Slice(worst, func(i, j int) bool {
		if worst[i].MinFreeDays != worst[j].MinFreeDays {
			return worst[i].MinFreeDays < worst[j].MinFreeDays
		}
		if worst[i].ExamCount != worst[j].ExamCount {
			return worst[i].ExamCount > worst[j].ExamCount
		}
		return worst[i].Name < worst[j].Name
	})
	if len(worst) > worstStudentsLimit {
		worst = worst[:worstStudentsLimit]
	}
	stat.WorstStudents = worst

	return stat, nil
}

// periodFilter reports whether an absolute time falls within the exam period
// [from, until] (inclusive of the whole Until day). If the bounds are unset it accepts
// everything, so a missing config never silently drops all exams.
func periodFilter(from, until time.Time) func(time.Time) bool {
	if from.IsZero() || until.IsZero() {
		return func(time.Time) bool { return true }
	}
	end := until.AddDate(0, 0, 1)
	return func(t time.Time) bool {
		return !t.Before(from) && t.Before(end)
	}
}

// capExamCount folds exam counts of 6 and above into a single top bucket.
func capExamCount(n int) int {
	if n >= 6 {
		return 6
	}
	return n
}

func share(count int, denom float64) float64 {
	if denom == 0 {
		return 0
	}
	return roundPercent(100 * float64(count) / denom)
}

func roundPercent(v float64) float64 {
	return float64(int(v*10+0.5)) / 10 // one decimal
}

func buildBuckets(counts map[string]int, total int) []*model.SpreadBucket {
	out := make([]*model.SpreadBucket, 0, len(spreadcalc.BucketOrder))
	for _, key := range spreadcalc.BucketOrder {
		out = append(out, &model.SpreadBucket{
			Key:   key,
			Label: spreadBucketLabels[key],
			Count: counts[key],
			Share: share(counts[key], float64(total)),
		})
	}
	return out
}

func buildExamCountBuckets(hist map[int]int, total int) []*model.CountBucket {
	keys := make([]int, 0, len(hist))
	for k := range hist {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	out := make([]*model.CountBucket, 0, len(keys))
	for _, k := range keys {
		label := pluralN(k, "Prüfung", "Prüfungen")
		if k >= 6 {
			label = "6+ Prüfungen"
		}
		out = append(out, &model.CountBucket{
			ExamCount: k,
			Label:     label,
			Students:  hist[k],
			Share:     share(hist[k], float64(total)),
		})
	}
	return out
}

// spreadProgAgg accumulates the per-program spread numbers.
type spreadProgAgg struct {
	students, multi int
	examSum         int
	freeDay         int // multi-exam students with min >= 1 free day
	sameDay         int // multi-exam students whose worst is same-day
	minSum          int
}

func buildProgramSpread(progs map[string]*spreadProgAgg) []*model.ProgramSpread {
	out := make([]*model.ProgramSpread, 0, len(progs))
	for name, pa := range progs {
		ps := &model.ProgramSpread{
			Program:               name,
			StudentCount:          pa.students,
			MultiExamStudentCount: pa.multi,
			LowSampleSize:         pa.multi < lowSampleThreshold,
		}
		if pa.students > 0 {
			ps.AvgExamsPerStudent = roundPercent(float64(pa.examSum) / float64(pa.students))
		}
		if pa.multi > 0 {
			ps.FreeDayShare = share(pa.freeDay, float64(pa.multi))
			ps.SameDayShare = share(pa.sameDay, float64(pa.multi))
			ps.AvgMinFreeDays = roundPercent(float64(pa.minSum) / float64(pa.multi))
		}
		out = append(out, ps)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StudentCount != out[j].StudentCount {
			return out[i].StudentCount > out[j].StudentCount
		}
		return out[i].Program < out[j].Program
	})
	return out
}

func buildWorstStudent(s *model.Student, sp spreadcalc.StudentSpread, times []spreadcalc.ExamTime, ancodes []int, info map[int]examInfo) *model.WorstStudent {
	exams := make([]*model.WorstStudentExam, 0, len(times))
	for i, ancode := range ancodes {
		durMin := int(times[i].End.Sub(times[i].Start).Minutes())
		exams = append(exams, &model.WorstStudentExam{
			Ancode:          ancode,
			Module:          info[ancode].module,
			Starttime:       times[i].Start,
			DurationMinutes: durMin,
		})
	}
	sort.Slice(exams, func(i, j int) bool { return exams[i].Starttime.Before(exams[j].Starttime) })
	key := spreadcalc.BucketKey(sp.MinGap)
	return &model.WorstStudent{
		Mtknr:       s.Mtknr,
		Name:        s.Name,
		Program:     s.Program,
		Group:       s.Group,
		ExamCount:   len(times),
		MinFreeDays: int(sp.MinGap),
		WorstLabel:  spreadBucketLabels[key],
		Exams:       exams,
	}
}

func meanInts(xs []int) float64 {
	if len(xs) == 0 {
		return 0
	}
	sum := 0
	for _, x := range xs {
		sum += x
	}
	return roundPercent(float64(sum) / float64(len(xs)))
}

func medianInts(xs []int) float64 {
	if len(xs) == 0 {
		return 0
	}
	sorted := make([]int, len(xs))
	copy(sorted, xs)
	sort.Ints(sorted)
	n := len(sorted)
	if n%2 == 1 {
		return float64(sorted[n/2])
	}
	return float64(sorted[n/2-1]+sorted[n/2]) / 2
}
