package plexams

import (
	"context"
	"sort"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/repeatcalc"
	"github.com/obcode/plexams.go/plexams/spreadcalc"
)

// worstStudentsLimit caps the drill-down list of the most tightly-scheduled students.
const worstStudentsLimit = 25

// lowSampleThreshold is the number of multi-exam students below which a program's
// shares rest on too small a base to be read as reliable percentages.
const lowSampleThreshold = 5

// maxRegularNonRepeatExams is the most non-repeat exams a student can have in a normal
// course of study; above it the extra registrations are repeat exams. The "regular"
// scope keeps only students at or below this, the meaningful headline population.
const maxRegularNonRepeatExams = 6

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
// Carter-style proximity index and a worst-off drill-down list. The figures are
// returned for two populations: the "regular" students (<= maxRegularNonRepeatExams
// non-repeat exams) and all students.
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
	for _, prog := range p.jointProgramNames(ctx) {
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
	externalByAncode := make(map[int]bool)
	for _, pe := range planEntries {
		if pe.Starttime == nil {
			continue
		}
		plannedSomewhere[pe.Ancode] = true
		if pe.External {
			externalByAncode[pe.Ancode] = true
		}
		if inPeriod(*pe.Starttime) {
			startByAncode[pe.Ancode] = *pe.Starttime
		}
	}

	excludePair := p.spreadPairExcluder(ctx, externalByAncode)

	// Build per-student records. The statistic covers the "regular" students (<=
	// maxRegularNonRepeatExams non-repeat exams — the most anyone can have in a normal
	// course of study); the repeat-heavy outliers barely move the aggregate, so they are
	// only summarized (excludedStudentCount + allFreeDayShare) rather than shown as a
	// second view. ZpaAncodes already holds only to-plan ZPA + external registrations
	// (not-to-plan and orphan Primuss regs are dropped upstream in PrepareStudentRegs).
	var regular []*studentSpreadRecord
	allStudents, allMulti, allFree := 0, 0, 0 // headline counters over ALL students
	for _, s := range students {
		if restrictProgram && !ownProgram[s.Program] {
			continue // not one of our (fully-known) students
		}
		studSem := repeatcalc.SemesterOf(s.Group)
		rec := &studentSpreadRecord{student: s}
		nonRepeat := 0
		for _, ancode := range s.ZpaAncodes {
			ei := info[ancode]
			if !repeatcalc.RepeatForStudent(studSem, ei.repeater, ei.minSem) {
				nonRepeat++
			}
			if start, ok := startByAncode[ancode]; ok {
				dur := durByAncode[ancode].forStudent(s.Mtknr)
				rec.times = append(rec.times, spreadcalc.ExamTime{Ancode: ancode, Start: start, End: start.Add(time.Duration(dur) * time.Minute)})
				rec.ancodes = append(rec.ancodes, ancode)
			} else if !plannedSomewhere[ancode] && ancode < externalAncodeBase {
				// an unplaced own exam (coverage caveat), not a placed-elsewhere external one
				rec.hasUnplanned = true
			}
		}
		rec.nExams = len(rec.times)
		if rec.nExams == 0 && !rec.hasUnplanned {
			continue // nothing to contribute
		}
		if rec.nExams >= 2 {
			rec.sp = spreadcalc.ComputeStudent(rec.times, examGap, notTooClose, excludePair)
		}

		// all-students headline (for the "outliers barely differ" note)
		if rec.nExams >= 1 {
			allStudents++
		}
		if rec.nExams >= 2 && len(rec.sp.Pairs) > 0 {
			allMulti++
			if rec.sp.MinGap >= 1 {
				allFree++
			}
		}

		if nonRepeat <= maxRegularNonRepeatExams {
			regular = append(regular, rec)
		}
	}

	stat := aggregateScope(regular, info)
	stat.MaxRegularNonRepeatExams = maxRegularNonRepeatExams
	stat.ExcludedStudentCount = allStudents - stat.StudentCount
	stat.AllFreeDayShare = share(allFree, float64(allMulti))
	stat.ExamGapMinutes = examGap
	stat.NotTooCloseMinutes = notTooClose
	return stat, nil
}

// studentSpreadRecord is one student's placed exams plus their precomputed spread,
// built once and then aggregated into one or both scopes.
type studentSpreadRecord struct {
	student      *model.Student
	times        []spreadcalc.ExamTime
	ancodes      []int
	sp           spreadcalc.StudentSpread // valid only when nExams >= 2
	nExams       int
	hasUnplanned bool
}

// aggregateScope turns a set of per-student records into the population's figures
// (shares, distributions, per-program breakdown, worst-off list). The caller fills in
// the scope-independent fields (thresholds, outlier note).
func aggregateScope(records []*studentSpreadRecord, info map[int]examInfo) *model.ExamSpreadStatistics {
	scope := &model.ExamSpreadStatistics{}

	studentBucket := make(map[string]int) // worst-case bucket per student
	pairBucket := make(map[string]int)    // every consecutive pair
	examCountHist := make(map[int]int)
	mins := make([]int, 0, len(records))
	var proximitySum float64
	progs := make(map[string]*spreadProgAgg)
	worst := make([]*model.WorstStudent, 0, len(records))

	for _, rec := range records {
		if rec.nExams == 0 {
			if rec.hasUnplanned {
				scope.StudentsWithUnplannedExams++
			}
			continue
		}
		s := rec.student
		scope.StudentCount++
		scope.TotalPlannedExams += rec.nExams
		if rec.hasUnplanned {
			scope.StudentsWithUnplannedExams++
		}
		if rec.nExams > scope.MaxExamsPerStudent {
			scope.MaxExamsPerStudent = rec.nExams
		}
		examCountHist[capExamCount(rec.nExams)]++

		pa := progs[s.Program]
		if pa == nil {
			pa = &spreadProgAgg{}
			progs[s.Program] = pa
		}
		pa.students++
		pa.examSum += rec.nExams

		if rec.nExams < 2 {
			continue // single exam: no gap to rate
		}
		sp := rec.sp
		if sp.MaxExamsPerDay >= 3 {
			scope.ThreeExamsOneDayCount++
		}
		if len(sp.Pairs) == 0 {
			continue // all pairs were spurious (foreign-foreign / same-slot) — no ratable gap
		}
		scope.MultiExamStudentCount++
		proximitySum += sp.ProximityCost
		mins = append(mins, int(sp.MinGap))

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

		worst = append(worst, buildWorstStudent(s, sp, rec.times, rec.ancodes, info))
	}

	// headline shares are a clean partition of the multi-exam students by their worst gap
	denom := float64(scope.MultiExamStudentCount)
	scope.ConflictShare = share(studentBucket[spreadcalc.KeyOverlap], denom)
	scope.SameDayShare = share(studentBucket[spreadcalc.KeySameDay], denom)
	scope.AdjacentDayShare = share(studentBucket[spreadcalc.KeyAdjacent], denom)
	scope.FreeDayShare = share(studentBucket[spreadcalc.KeyOneFree]+studentBucket[spreadcalc.KeyTwoFree]+studentBucket[spreadcalc.KeyThreePlus], denom)
	if scope.StudentCount > 0 {
		scope.AvgExamsPerStudent = float64(scope.TotalPlannedExams) / float64(scope.StudentCount)
	}
	if scope.MultiExamStudentCount > 0 {
		scope.AvgProximityCost = proximitySum / denom
		scope.AvgMinFreeDays = meanInts(mins)
		scope.MedianMinFreeDays = medianInts(mins)
	}

	scope.StudentBuckets = buildBuckets(studentBucket, scope.MultiExamStudentCount)
	pairTotal := 0
	for _, c := range pairBucket {
		pairTotal += c
	}
	scope.PairBuckets = buildBuckets(pairBucket, pairTotal)
	scope.ExamCountBuckets = buildExamCountBuckets(examCountHist, scope.StudentCount)
	scope.ByProgram = buildProgramSpread(progs)

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
	scope.WorstStudents = worst

	return scope
}

// spreadPairExcluder builds the predicate that drops a student's exam pair from the
// gap statistics when it is not a real scheduling problem: two exams of other faculties
// (neither ours to resolve — mirrors ValidateConflicts, which drops foreign-foreign
// pairs), or two exams declared same-slot / can-share-slot (a student may not sit both,
// so the registration is spurious). externalByAncode marks ancodes whose time was set
// externally (PlanEntry.External).
func (p *Plexams) spreadPairExcluder(ctx context.Context, externalByAncode map[int]bool) func(a, b int) bool {
	constraints, _ := p.ConstraintsMap(ctx)
	foreign := func(ancode int) bool {
		if ancode >= externalAncodeBase || externalByAncode[ancode] {
			return true
		}
		c := constraints[ancode]
		return c != nil && c.NotPlannedByMe
	}
	canShare := make(map[[2]int]bool)
	if pairs, err := p.dbClient.CanShareSlotPairs(ctx); err == nil {
		for _, pr := range pairs {
			canShare[[2]int{pr[0], pr[1]}] = true
		}
	}
	ssRoot := p.sameSlotGroups(ctx)
	return func(a, b int) bool {
		if foreign(a) && foreign(b) {
			return true // two exams of other faculties — not ours to resolve
		}
		x, y := a, b
		if x > y {
			x, y = y, x
		}
		if canShare[[2]int{x, y}] {
			return true // declared "may share a slot" — a student can't legitimately have both
		}
		if r, ok := ssRoot[a]; ok && r == ssRoot[b] {
			return true // same-slot group — the two exams run simultaneously
		}
		return false
	}
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
