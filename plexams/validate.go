package plexams

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	set "github.com/deckarep/golang-set/v2"
	"github.com/logrusorgru/aurora"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/anny"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// TODO: Validate if all NTAs have MTKNR

var knownConflictsCount = 0

// studentPair identifies one student's conflict between two exams, with the ancodes
// normalized (ancode1 < ancode2) so lookups are order-independent. It keys the set of
// conflicts the user has accepted (registration is wrong, student writes only one, …).
type studentPair struct {
	mtknr            string
	ancode1, ancode2 int
}

func acceptedKey(mtknr string, a, b int) studentPair {
	if a > b {
		a, b = b, a
	}
	return studentPair{mtknr: mtknr, ancode1: a, ancode2: b}
}

type conflictingAncodes struct {
	ancode1 int
	ancode2 int
}
type problemWithStudents struct {
	problem  string
	students []string // affected students that are a real problem
	accepted []string // affected students whose conflict the user has accepted (→ info)
}

// Conflict severity by proximity of two of a student's exams. Same slot is a hard
// clash (a student cannot sit both at once) → error. Adjacent slots (back-to-back, no
// break) are undesirable → warning. Same day (not adjacent) is usually acceptable →
// info. These strings are also the human labels streamed in the report.
const (
	conflictSameSlot = "same slot"
	conflictAdjacent = "adjacent slot"
	conflictSameDay  = "same day"
)

// conflictSeverityRank orders the conflict kinds by their inherent (original) severity,
// most severe first: same slot (0) > adjacent (1) > same day (2).
func conflictSeverityRank(problem string) int {
	switch problem {
	case conflictSameSlot:
		return 0
	case conflictAdjacent:
		return 1
	default: // conflictSameDay
		return 2
	}
}

// conflictLevel applies the severity rule: everything the user allows is only info — a
// pair-level allowance (allowed: sameSlot constraint / canShareSlot) or all affected
// students explicitly accepted (real == 0). Otherwise grade by proximity: same slot =
// error, adjacent = warning, same day = info.
func conflictLevel(problem string, real int, allowed bool) model.ValidationLevel {
	if allowed || real == 0 {
		return model.ValidationLevelInfo
	}
	switch problem {
	case conflictSameSlot:
		return model.ValidationLevelError
	case conflictAdjacent:
		return model.ValidationLevelWarning
	default: // conflictSameDay
		return model.ValidationLevelInfo
	}
}

// levelRank orders finding levels most severe first: error (0) > warning (1) > info (2).
func levelRank(level model.ValidationLevel) int {
	switch level {
	case model.ValidationLevelError:
		return 0
	case model.ValidationLevelWarning:
		return 1
	default: // info
		return 2
	}
}

func (p *Plexams) ValidateConflicts(onlyPlannedByMe bool, ancode int, reporter Reporter) (*model.ValidationReport, error) {
	knownConflictsCount = 0
	ctx := context.Background()
	v := newValidation(reporter, "conflicts", "validating conflicts")

	if ok, err := p.planGenerated(ctx); err != nil {
		return nil, err
	} else if !ok {
		return v.skip(skipNoPlan), nil
	}

	validationMessages := make(map[conflictingAncodes]*problemWithStudents)

	v.step("get planned ancodes")
	planAncodeEntries, err := p.dbClient.PlannedAncodes(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get plan entries")
		return nil, err
	}

	// foreign = exams we do not plan ourselves: not-planned-by-me constraint, an external
	// (fixed) time, or an auto-assigned external ancode (>= externalAncodeBase). Matches
	// examplan's "foreign" definition. A conflict between two foreign exams is not ours to
	// resolve and is dropped entirely.
	foreignAncodes := set.NewSet[int]()
	for _, entry := range planAncodeEntries {
		constraints, err := p.dbClient.GetConstraintsForAncode(ctx, entry.Ancode)
		if err != nil {
			log.Error().Err(err).Int("ancode", entry.Ancode).Msg("cannot get constraints for ancode")
			return nil, err
		}
		if (constraints != nil && constraints.NotPlannedByMe) || entry.External || entry.Ancode >= externalAncodeBase {
			foreignAncodes.Add(entry.Ancode)
		}
	}

	v.step("get student regs")
	students, err := p.StudentRegsPerStudentPlanned(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get student registries per student")
		return nil, err
	}

	// Conflicts the user has accepted are not real problems: they are still shown, but
	// downgraded to info. Sources: the GUI/DB per-student decisions (acceptStudentConflict)
	// and, for backwards compatibility, the legacy knownConflicts in the semester YAML.
	accepted := set.NewSet[studentPair]()

	v.step("get accepted conflicts")
	if decisions, err := p.dbClient.StudentConflictDecisions(ctx); err != nil {
		log.Error().Err(err).Msg("cannot get student conflict decisions")
	} else {
		for _, d := range decisions {
			if d.Decision == model.ConflictDecisionAccept {
				accepted.Add(acceptedKey(d.Mtknr, d.Ancode1, d.Ancode2))
			}
		}
	}
	knownConflictsConf := viper.Get("knownConflicts.studentRegs")
	if knownConflictsConf != nil {
		knownConflictsSlice := knownConflictsConf.([]interface{})
		for _, knownConflict := range knownConflictsSlice {
			knownConflictSlice := knownConflict.([]interface{})
			mtknr, _ := knownConflictSlice[0].(string)
			a, errA := strconv.Atoi(fmt.Sprint(knownConflictSlice[1]))
			b, errB := strconv.Atoi(fmt.Sprint(knownConflictSlice[2]))
			if errA != nil || errB != nil {
				log.Debug().Interface("knownConflict", knownConflict).Msg("skipping unparseable YAML knownConflict")
				continue
			}
			accepted.Add(acceptedKey(mtknr, a, b))
		}
	}

	log.Debug().Int("count", accepted.Cardinality()).Interface("accepted", accepted).Msg("found accepted conflicts")

	v.step("validating students")
	for _, student := range students {
		p.validateStudentReg(student, planAncodeEntries, foreignAncodes, onlyPlannedByMe,
			accepted, ancode, &validationMessages)
	}

	conflictingAncodesSlice, normalizedValidationMessages := p.sortConflictingAncodes(validationMessages)

	// Pair-level allowances: exam pairs the user permits to share a slot, so any same-slot
	// (or closer) proximity between them is not a problem, only info. Two sources: the
	// sameSlot constraint (exams that MUST run together) and the canShareSlot
	// ("canBeSameSlot") declarations. Keyed by the normalized (ancode1 < ancode2) pair;
	// conflictingAncodes are already built in that order.
	sameSlotConstraintPairs := set.NewSet[[2]int]()
	canSharePairs := set.NewSet[[2]int]()
	normPair := func(a, b int) [2]int {
		if a > b {
			a, b = b, a
		}
		return [2]int{a, b}
	}
	if cmap, err := p.ConstraintsMap(ctx); err != nil {
		log.Error().Err(err).Msg("cannot get constraints for sameSlot pairs")
	} else {
		for a, c := range cmap {
			if c == nil {
				continue
			}
			for _, other := range c.SameSlot {
				sameSlotConstraintPairs.Add(normPair(a, other))
			}
		}
	}
	if pairs, err := p.dbClient.CanShareSlotPairs(ctx); err != nil {
		log.Error().Err(err).Msg("cannot get canShareSlot pairs")
	} else {
		for _, pr := range pairs {
			canSharePairs.Add(normPair(pr[0], pr[1]))
		}
	}
	// allowedReason reports why a pair's shared slot is permitted (→ info), if at all.
	// sortConflictingAncodes may order ca by exam time, so normalize the key here.
	allowedReason := func(ca conflictingAncodes) (string, bool) {
		key := normPair(ca.ancode1, ca.ancode2)
		switch {
		case sameSlotConstraintPairs.Contains(key):
			return "sameSlot-Constraint", true
		case canSharePairs.Contains(key):
			return "canShareSlot", true
		default:
			return "", false
		}
	}

	// Grade one finding per conflicting exam pair. Everything the user allows is only
	// info: a pair-level allowance (sameSlot constraint / canShareSlot) or all affected
	// students explicitly accepted. Otherwise grade by proximity: same slot = error,
	// adjacent = warning, same day = info.
	type gradedConflict struct {
		ca       conflictingAncodes
		level    model.ValidationLevel
		proxRank int
		message  string
	}
	graded := make([]gradedConflict, 0, len(conflictingAncodesSlice))
	for _, ca := range conflictingAncodesSlice {
		problem := normalizedValidationMessages[ca]
		real, acc := len(problem.students), len(problem.accepted)
		reason, isAllowed := allowedReason(ca)
		g := gradedConflict{
			ca:       ca,
			proxRank: conflictSeverityRank(problem.problem),
			level:    conflictLevel(problem.problem, real, isAllowed),
		}
		switch {
		case isAllowed:
			g.message = fmt.Sprintf("%s (%s): %d student(s) affected between exam %d and %d",
				problem.problem, reason, real+acc, ca.ancode1, ca.ancode2)
		case real == 0: // all affected students explicitly accepted
			g.message = fmt.Sprintf("%s (akzeptiert): %d student(s) affected between exam %d and %d",
				problem.problem, acc, ca.ancode1, ca.ancode2)
		default:
			suffix := ""
			if acc > 0 {
				suffix = fmt.Sprintf(" (+%d akzeptiert)", acc)
			}
			g.message = fmt.Sprintf("%s: %d student(s) affected between exam %d and %d%s",
				problem.problem, real, ca.ancode1, ca.ancode2, suffix)
		}
		graded = append(graded, g)
	}
	// Most severe first: primary by original proximity (same slot, then adjacent, then
	// same day), secondary by effective level so real problems lead within a proximity
	// group. Stable, so the time order from sortConflictingAncodes is kept within ties.
	sort.SliceStable(graded, func(i, j int) bool {
		if graded[i].proxRank != graded[j].proxRank {
			return graded[i].proxRank < graded[j].proxRank
		}
		return levelRank(graded[i].level) < levelRank(graded[j].level)
	})
	for _, g := range graded {
		v.add(g.level, ref{Ancode: ptr(g.ca.ancode1), RelatedAncodes: []int{g.ca.ancode2}}, "%s", g.message)
	}

	if len(validationMessages) > 0 {
		mucdaiPrograms := p.mucdaiProgramNames(ctx)
		mucdaiprogram := make(map[int]string)
		for _, prog := range mucdaiPrograms {
			if base, ok := p.externalExamsBaseForProgram(ctx, prog); ok {
				mucdaiprogram[base] = prog
			}
		}

		errs, warns, infos := v.counts()
		summary := aurora.Sprintf(aurora.Yellow("%d accepted conflicts; %d error(s), %d warning(s), %d info(s)"),
			knownConflictsCount, errs, warns, infos)
		if errs > 0 {
			v.reporter.StopProgressFail(summary)
		} else {
			v.reporter.StopProgress(summary)
		}

		// Stream the copy-pasteable knownConflicts YAML snippet for the pairs that are NOT
		// yet accepted, like the CLI (so they can be accepted).
		v.reporter.Println("")
		v.reporter.Println("knownConflicts:")
		v.reporter.Println("  studentRegs:")
		for _, conflictingAncodes := range conflictingAncodesSlice {
			problemWithStudents := normalizedValidationMessages[conflictingAncodes]
			if _, ok := allowedReason(conflictingAncodes); ok {
				continue // pair may share a slot (sameSlot constraint / canShareSlot): nothing to accept
			}
			if len(problemWithStudents.students) == 0 {
				continue // fully accepted: nothing left to accept
			}
			exam1, err := p.PlannedExam(ctx, conflictingAncodes.ancode1)
			if err != nil {
				log.Debug().Err(err).Msg("cannot get planned exam")
				continue
			}
			exam2, err := p.PlannedExam(ctx, conflictingAncodes.ancode2)
			if err != nil {
				log.Debug().Err(err).Msg("cannot get planned exam")
				continue
			}

			oneIsMucdai := false
			if exam1.Ancode > 999 || exam2.Ancode > 999 {
				oneIsMucdai = true
			}

			log.Debug().Interface("exam1", exam1).Interface("exam2", exam2).Msg("found conflicting exams")
			v.reporter.Printf("%s", aurora.Sprintf(aurora.Red("\n    # %s"), problemWithStudents.problem))
			for _, exam := range []*model.PlannedExam{exam1, exam2} {
				repeater := ""
				if exam.ZpaExam.IsRepeaterExam {
					repeater = "-- Wiederholungsprüfung"
				}

				ancode := exam.Ancode
				ancodeStr := fmt.Sprintf("%3d", ancode)
				zpaAncode := ""
				if oneIsMucdai {
					ancodeStr = fmt.Sprintf("%6d", ancode)
					zpaAncode = "           "

					if ancode > 999 {
						primussAncode := ancode % 1000
						base := ancode - primussAncode
						program := mucdaiprogram[base]
						ancodeStr = fmt.Sprintf("%s/%d", program, primussAncode)
					} else {
						for _, primussExam := range exam.PrimussExams {
							if primussExam.Exam.AnCode != exam.Ancode {
								ancodeStr = fmt.Sprintf("%6d", primussExam.Exam.AnCode)
								zpaAncode = fmt.Sprintf(" (ZPA: %d)", exam.Ancode)
								break
							}
						}
					}
				}

				planEntry := exam.PlanEntry
				time := p.getSlotTime(planEntry.DayNumber, planEntry.SlotNumber)
				if planEntry.Starttime != nil {
					time = *planEntry.Starttime
				}

				v.reporter.Printf("%s", aurora.Sprintf(aurora.Red("    # %s: %s. %s (%s): %s %s %s"),
					time.Format("02.01.06, 15:04 Uhr"),
					aurora.Magenta(ancodeStr),
					aurora.Cyan(exam.ZpaExam.Module), aurora.Cyan(exam.ZpaExam.MainExamer),
					aurora.Yellow(exam.ZpaExam.Groups),
					aurora.Cyan(repeater),
					zpaAncode,
				))
			}
			for _, studentStr := range problemWithStudents.students {
				v.reporter.Printf("%s", studentStr)
			}
		}

	} else {
		v.reporter.StopProgress(aurora.Sprintf(aurora.Green("%d known conflicts, no further problems found"),
			knownConflictsCount))
	}

	return v.report(), nil
}

func (plexams *Plexams) sortConflictingAncodes(validationMessages map[conflictingAncodes]*problemWithStudents) ([]conflictingAncodes, map[conflictingAncodes]*problemWithStudents) {
	ctx := context.Background()
	ca := make([]conflictingAncodes, 0, len(validationMessages))
	examCache := make(map[int]*model.PlannedExam)
	timeCache := make(map[int]time.Time)
	normalizedValidationMessages := make(map[conflictingAncodes]*problemWithStudents, len(validationMessages))
	pairStartTimes := make(map[conflictingAncodes][2]time.Time, len(validationMessages))

	getExamStartTime := func(ancode int) (time.Time, error) {
		if cachedTime, ok := timeCache[ancode]; ok {
			return cachedTime, nil
		}

		exam, ok := examCache[ancode]
		if !ok {
			plannedExam, err := plexams.PlannedExam(ctx, ancode)
			if err != nil {
				return time.Time{}, err
			}
			exam = plannedExam
			examCache[ancode] = exam
		}

		planEntry := exam.PlanEntry
		startTime := plexams.getSlotTime(planEntry.DayNumber, planEntry.SlotNumber)
		if planEntry.Starttime != nil {
			startTime = *planEntry.Starttime
		}

		timeCache[ancode] = startTime
		return startTime, nil
	}

	for c, problem := range validationMessages {
		exam1Time, err := getExamStartTime(c.ancode1)
		if err != nil {
			log.Debug().Err(err).Int("ancode", c.ancode1).Msg("cannot get planned exam for sorting")
			ca = append(ca, c)
			normalizedValidationMessages[c] = problem
			pairStartTimes[c] = [2]time.Time{}
			continue
		}
		exam2Time, err := getExamStartTime(c.ancode2)
		if err != nil {
			log.Debug().Err(err).Int("ancode", c.ancode2).Msg("cannot get planned exam for sorting")
			ca = append(ca, c)
			normalizedValidationMessages[c] = problem
			pairStartTimes[c] = [2]time.Time{exam1Time, time.Time{}}
			continue
		}

		normalized := c
		if exam2Time.Before(exam1Time) || (exam1Time.Equal(exam2Time) && c.ancode2 < c.ancode1) {
			normalized = conflictingAncodes{ancode1: c.ancode2, ancode2: c.ancode1}
			exam1Time, exam2Time = exam2Time, exam1Time
		}
		ca = append(ca, normalized)
		normalizedValidationMessages[normalized] = problem
		pairStartTimes[normalized] = [2]time.Time{exam1Time, exam2Time}
	}

	sort.SliceStable(ca, func(i, j int) bool {
		leftTimes := pairStartTimes[ca[i]]
		rightTimes := pairStartTimes[ca[j]]
		if !leftTimes[0].Equal(rightTimes[0]) {
			return leftTimes[0].Before(rightTimes[0])
		}
		if !leftTimes[1].Equal(rightTimes[1]) {
			return leftTimes[1].Before(rightTimes[1])
		}
		if ca[i].ancode1 != ca[j].ancode1 {
			return ca[i].ancode1 < ca[j].ancode1
		}
		return ca[i].ancode2 < ca[j].ancode2
	})

	return ca, normalizedValidationMessages
}

func (plexams *Plexams) validateStudentReg(student *model.Student, planAncodeEntries []*model.PlanEntry,
	foreignAncodes set.Set[int], onlyPlannedByMe bool, accepted set.Set[studentPair], ancode int,
	validationMessages *map[conflictingAncodes]*problemWithStudents) {
	log.Debug().Str("name", student.Name).Str("mtknr", student.Mtknr).Msg("checking regs for student")

	planAncodeEntriesForStudent := make([]*model.PlanEntry, 0)
	for _, ancode := range student.Regs {
		for _, planEntry := range planAncodeEntries {
			if ancode == planEntry.Ancode {
				planAncodeEntriesForStudent = append(planAncodeEntriesForStudent, planEntry)
			}
		}
	}

	if len(planAncodeEntriesForStudent) == 0 {
		log.Debug().Str("name", student.Name).Str("mtknr", student.Mtknr).Msg("no exam for student in plan")
		return
	}

	log.Debug().Str("name", student.Name).Str("mtknr", student.Mtknr).
		Int("count", len(planAncodeEntriesForStudent)).
		Msg("found exams for student in plan")

	p := planAncodeEntriesForStudent
	for i := 0; i < len(planAncodeEntriesForStudent); i++ {
		for j := i + 1; j < len(planAncodeEntriesForStudent); j++ {
			if p[i].DayNumber == p[j].DayNumber &&
				p[i].SlotNumber == p[j].SlotNumber &&
				p[i].Ancode == p[j].Ancode {
				continue
			}
			// A conflict between two exams we do not plan (external / not planned by me)
			// is not ours to resolve — never report it. With onlyPlannedByMe, restrict to
			// conflicts purely among our own exams (drop any pair with a foreign side).
			iForeign := foreignAncodes.Contains(p[i].Ancode)
			jForeign := foreignAncodes.Contains(p[j].Ancode)
			if iForeign && jForeign {
				log.Debug().Int("ancode1", p[i].Ancode).Int("ancode2", p[j].Ancode).
					Msg("both ancodes not planned by me (foreign)")
				continue
			}
			if onlyPlannedByMe && (iForeign || jForeign) {
				continue
			}
			if ancode != 0 && p[i].Ancode != ancode && p[j].Ancode != ancode {
				continue
			}
			isAccepted := accepted.Contains(acceptedKey(student.Mtknr, p[i].Ancode, p[j].Ancode))
			if isAccepted {
				knownConflictsCount++
			}

			problem := ""
			// same slot
			if p[i].DayNumber == p[j].DayNumber &&
				p[i].SlotNumber == p[j].SlotNumber {
				problem = conflictSameSlot
			} else
			// adjacent slots
			if p[i].DayNumber == p[j].DayNumber &&
				(p[i].SlotNumber+1 == p[j].SlotNumber ||
					p[i].SlotNumber-1 == p[j].SlotNumber) {
				problem = conflictAdjacent
			} else
			// same day
			if p[i].DayNumber == p[j].DayNumber {
				problem = conflictSameDay
			}

			if problem != "" {
				smallerAncode := p[i].Ancode
				largerAncode := p[j].Ancode
				if p[i].Ancode > p[j].Ancode {
					smallerAncode = p[j].Ancode
					largerAncode = p[i].Ancode
				}

				conflictingAncodes := conflictingAncodes{
					ancode1: smallerAncode,
					ancode2: largerAncode,
				}

				validationMessageForProblem, ok := (*validationMessages)[conflictingAncodes]
				if !ok {
					validationMessageForProblem = &problemWithStudents{
						problem:  problem,
						students: make([]string, 0),
					}
				}
				studentLine := aurora.Sprintf(aurora.Yellow("    - [\"%s\", \"%d\", \"%d\"] # %s (%s%s / %s)"),
					aurora.Magenta(student.Mtknr), aurora.Magenta(p[i].Ancode), aurora.Magenta(p[j].Ancode),
					aurora.Cyan(student.Name), aurora.Cyan(student.Program), aurora.Cyan(student.Group), aurora.Cyan(student.Mtknr),
				)
				if isAccepted {
					validationMessageForProblem.accepted = append(validationMessageForProblem.accepted, studentLine)
				} else {
					validationMessageForProblem.students = append(validationMessageForProblem.students, studentLine)
				}
				(*validationMessages)[conflictingAncodes] = validationMessageForProblem
			}
		}
	}
}

func (p *Plexams) ValidateConstraints(reporter Reporter) (*model.ValidationReport, error) {
	ctx := context.Background()
	v := newValidation(reporter, "constraints", "validating constraints")

	if ok, err := p.planGenerated(ctx); err != nil {
		return nil, err
	} else if !ok {
		return v.skip(skipNoPlan), nil
	}

	v.step("get constraints")
	constraints, err := p.Constraints(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get constraints")
	}

	constraintsMap := make(map[int]*model.Constraints)
	for _, constraint := range constraints {
		constraintsMap[constraint.Ancode] = constraint
	}

	v.step("get booked entries")
	annyRoomBookings, err := p.ExahmRoomsFromAnnyBookings(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get entries from anny_bookings")
		return nil, err
	}

	for _, constraint := range constraints {
		v.step("check constraints for exam %d", constraint.Ancode)
		slot, err := p.SlotForAncode(ctx, constraint.Ancode)
		if err != nil {
			log.Error().Err(err).Int("ancode", constraint.Ancode).Msg("cannot get slot for ancode")
		}

		if slot == nil {
			continue
		}

		for _, otherAncode := range constraint.SameSlot {
			log.Debug().Int("ancode", constraint.Ancode).Int("other ancode", otherAncode).Msg("checking same slot")
			otherSlot, err := p.SlotForAncode(ctx, otherAncode)
			if err != nil {
				log.Error().Err(err).Int("ancode", otherAncode).Msg("cannot get slot for other ancode")
			}

			if otherSlot == nil {
				otherConstraint, ok := constraintsMap[otherAncode]
				if ok && otherConstraint.NotPlannedByMe {
					continue
				}

				v.errorf(ref{Ancode: ptr(constraint.Ancode), RelatedAncodes: []int{otherAncode}},
					"Exams %d and %d must be in the same slot, are %v and %v", constraint.Ancode, otherAncode, slot, otherSlot)
				continue
			}

			if *slot != *otherSlot {
				v.errorf(ref{Ancode: ptr(constraint.Ancode), RelatedAncodes: []int{otherAncode}},
					"Exams %d and %d must be in the same slot, are %v and %v", constraint.Ancode, otherAncode, slot, otherSlot)
			}
		}

		if constraint.FixedDay != nil {
			log.Debug().Int("ancode", constraint.Ancode).Msg("FIXME: FixedDay not validated yet")
		}

		if constraint.FixedTime != nil {
			log.Debug().Int("ancode", constraint.Ancode).Msg("checking fixed time")
			fixed := constraint.FixedTime
			if fixed.Day() != slot.Starttime.Day() ||
				fixed.Month() != slot.Starttime.Month() ||
				fixed.Hour() != slot.Starttime.Hour() ||
				fixed.Minute() != slot.Starttime.Minute() {
				v.errorf(ref{Ancode: ptr(constraint.Ancode)},
					"Exam %d has fixed slot %s, is %s", constraint.Ancode, fixed.Format("02.01.06 15:04"), constraint.FixedTime.Format("02.01.06 15:04"))
			}
		}

		for _, day := range constraint.ExcludeDays {
			if day.Equal(time.Date(slot.Starttime.Year(), slot.Starttime.Month(), slot.Starttime.Day(), 0, 0, 0, 0, time.Local)) {
				v.errorf(ref{Ancode: ptr(constraint.Ancode)},
					"Exam %d planned on excluded day %s", constraint.Ancode, day.Format("02.01.06"))
			}
		}

		if len(constraint.PossibleDays) > 0 {
			possibleDaysOk := false
			var dayPlanned *time.Time
			for _, day := range constraint.PossibleDays {
				if day.Equal(time.Date(slot.Starttime.Year(), slot.Starttime.Month(), slot.Starttime.Day(), 0, 0, 0, 0, time.Local)) {
					possibleDaysOk = true
					dayPlanned = day
					break
				}
			}
			if !possibleDaysOk {
				dayStr := "-"
				if dayPlanned != nil {
					dayStr = dayPlanned.Format("02.01.06")
				}
				v.errorf(ref{Ancode: ptr(constraint.Ancode)},
					"Exam %d planned on day %s which is not a possible day", constraint.Ancode, dayStr)
			}
		}

		if constraint.RoomConstraints != nil && (constraint.RoomConstraints.Exahm || constraint.RoomConstraints.Seb) {
			if !anny.RoomBookedDuringExamTime(annyRoomBookings, slot) {
				v.errorf(ref{Ancode: ptr(constraint.Ancode)},
					"Exam %d planned at %s, but no room booked", constraint.Ancode, slot.Starttime.Format("02.01.06 15:04"))
			}
		}

		if constraint.RoomConstraints != nil && len(constraint.RoomConstraints.AllowedRooms) > 0 {
			plannedRooms, err := p.dbClient.PlannedRoomsForAncode(ctx, constraint.Ancode)
			if err != nil {
				log.Error().Err(err).Int("ancode", constraint.Ancode).Msg("cannot get rooms for ancode")
			}
			allowedRooms := set.NewSet[string]()
			for _, room := range constraint.RoomConstraints.AllowedRooms {
				allowedRooms.Add(room)
			}
			for _, room := range plannedRooms {
				if !allowedRooms.Contains(room.RoomName) {
					v.errorf(ref{Ancode: ptr(constraint.Ancode), Room: ptr(room.RoomName)},
						"Exam %d planned in room %s, but allowed rooms are %s", constraint.Ancode, room.RoomName, constraint.RoomConstraints.AllowedRooms)
				}
			}
			if len(plannedRooms) == 0 {
				v.errorf(ref{Ancode: ptr(constraint.Ancode)},
					"Exam %d planned in no room, but allowed rooms are %s", constraint.Ancode, constraint.RoomConstraints.AllowedRooms)
			}
		}
	}

	return v.finish(), nil
}
