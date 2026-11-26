package plexams

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	set "github.com/deckarep/golang-set/v2"
	"github.com/logrusorgru/aurora"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/anny"
	"github.com/obcode/plexams.go/plexams/conflictcalc"
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

// Conflict severity by time proximity of two of a student's exams. An overlap (their
// exams incl. NTA time collide, or leave less than the travel/break buffer) is a hard
// clash → error. "Too close" (same day, start times closer than notTooCloseMinutes) is
// undesirable → warning. Same day (far enough) is usually acceptable → info. These
// strings are also the human labels streamed in the report.
const (
	conflictOverlap  = "overlap"
	conflictTooClose = "too close"
	conflictSameDay  = "same day"
)

// conflictText names a conflict kind for the planner.
func conflictText(problem string, notTooCloseMinutes int) string {
	switch problem {
	case conflictOverlap:
		return "Zeitkonflikt (die Prüfungen überschneiden sich oder folgen ohne die nötige Pause aufeinander)"
	case conflictTooClose:
		return fmt.Sprintf("Zu nah (selber Tag, Beginn weniger als %d Minuten auseinander)", notTooCloseMinutes)
	default:
		return "Selber Tag"
	}
}

// conflictHint says what to do about a conflict that is neither allowed nor accepted.
// Same day needs nothing: it is only info.
func conflictHint(problem string) string {
	switch problem {
	case conflictOverlap:
		return "Eine der beiden Prüfungen im Prüfungsplan verschieben. Schreiben die Betroffenen sie nicht wirklich beide (z. B. falsche Anmeldung), unter „Terminplan generieren“ den Konflikt je Studierende:n akzeptieren oder „darf zeitgleich“ setzen."
	case conflictTooClose:
		return "Wenn möglich eine der beiden Prüfungen auf einen anderen Tag oder weiter auseinander legen. Ist der Abstand vertretbar (z. B. bei Wiederholungsprüfungen), den Konflikt unter „Terminplan generieren“ je Studierende:n akzeptieren."
	default:
		return ""
	}
}

// conflictSeverityRank orders the conflict kinds by their inherent (original) severity,
// most severe first: overlap (0) > too close (1) > same day (2).
func conflictSeverityRank(problem string) int {
	switch problem {
	case conflictOverlap:
		return 0
	case conflictTooClose:
		return 1
	default: // conflictSameDay
		return 2
	}
}

// conflictLevel applies the severity rule: everything the user allows is only info — a
// pair-level allowance (allowed: sameSlot constraint / canShareSlot) or all affected
// students explicitly accepted (real == 0). Otherwise grade by proximity: overlap =
// error, too close = warning, same day = info.
func conflictLevel(problem string, real int, allowed bool) model.ValidationLevel {
	if allowed || real == 0 {
		return model.ValidationLevelInfo
	}
	switch problem {
	case conflictOverlap:
		return model.ValidationLevelError
	case conflictTooClose:
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
	v := newValidation(reporter, "conflicts", "prüfe die Konflikte der Studierenden")

	if ok, err := p.planGenerated(ctx); err != nil {
		return nil, err
	} else if !ok {
		return v.skip(skipNoPlan), nil
	}

	validationMessages := make(map[conflictingAncodes]*problemWithStudents)

	v.step("lese die geplanten Prüfungen")
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

	v.step("lese die Anmeldungen")
	students, err := p.StudentRegsPerStudentPlanned(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get student registries per student")
		return nil, err
	}

	// Conflicts the user has accepted are not real problems: they are still shown, but
	// downgraded to info. Sources: the GUI/DB per-student decisions (acceptStudentConflict)
	// and, for backwards compatibility, the legacy knownConflicts in the semester YAML.
	accepted := set.NewSet[studentPair]()

	v.step("lese die akzeptierten Konflikte")
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

	durByAncode := p.examDurationsByAncode(ctx)

	// per-exam campus, so a conflict between two exams at different campuses uses the larger
	// cross-campus travel buffer (a tighter placement is impossible even without a time-window
	// overlap). Built once here; nil is fine (all same/default campus).
	cmap, cmapErr := p.ConstraintsMap(ctx)
	if cmapErr != nil {
		log.Error().Err(cmapErr).Msg("cannot get constraints for conflict locations")
	}
	locByAncode := make(map[int]string, len(cmap))
	for a, c := range cmap {
		locByAncode[a] = locationOf(c)
	}

	v.step("prüfe je Studierende:n die Abstände der Prüfungen")
	for _, student := range students {
		p.validateStudentReg(student, planAncodeEntries, foreignAncodes, onlyPlannedByMe,
			accepted, ancode, durByAncode, locByAncode, &validationMessages)
	}

	conflictingAncodesSlice, normalizedValidationMessages := p.sortConflictingAncodes(validationMessages)
	examNames := p.newExamNamer(ctx, planAncodeEntries)

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
	for a, c := range cmap {
		if c == nil {
			continue
		}
		for _, other := range c.SameSlot {
			sameSlotConstraintPairs.Add(normPair(a, other))
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
			return "Vorgabe „zur selben Zeit wie“", true
		case canSharePairs.Contains(key):
			return "„darf zeitgleich“", true
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
		hint     string
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
		names := examNames.pair(ca.ancode1, ca.ancode2)
		switch {
		case isAllowed:
			g.message = fmt.Sprintf("Erlaubt (%s): %s. Betroffen: %d Studierende.",
				reason, names, real+acc)
		case real == 0: // all affected students explicitly accepted
			g.message = fmt.Sprintf("Akzeptiert: %s. Alle %d Betroffenen sind akzeptiert.",
				names, acc)
		default:
			suffix := ""
			if acc > 0 {
				suffix = fmt.Sprintf(" Weitere %d sind akzeptiert.", acc)
			}
			g.message = fmt.Sprintf("%s: %d Studierende schreiben %s.%s",
				conflictText(problem.problem, p.semesterConfig.NotTooCloseMinutes), real, names, suffix)
			g.hint = conflictHint(problem.problem)
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
		v.add(g.level, ref{Ancode: ptr(g.ca.ancode1), RelatedAncodes: []int{g.ca.ancode2}, Hint: g.hint}, "%s", g.message)
	}

	if len(validationMessages) > 0 {
		errs, warns, infos := v.counts()
		summary := aurora.Sprintf(aurora.Yellow("%d Fehler, %d Warnung(en), %d Hinweis(e); %d Konflikte einzelner Studierender sind akzeptiert"),
			errs, warns, infos, knownConflictsCount)
		if errs > 0 {
			v.reporter.StopProgressFail(summary)
		} else {
			v.reporter.StopProgress(summary)
		}

		// Who is affected, per pair that is neither allowed nor fully accepted. A
		// conflict is accepted per student on "Terminplan generieren".
		for _, ca := range conflictingAncodesSlice {
			problem := normalizedValidationMessages[ca]
			if _, ok := allowedReason(ca); ok || len(problem.students) == 0 {
				continue
			}
			v.reporter.Println("")
			v.reporter.Printf("%s", aurora.Sprintf(aurora.Red("%s: %s"),
				conflictText(problem.problem, p.semesterConfig.NotTooCloseMinutes), examNames.pair(ca.ancode1, ca.ancode2)))
			for _, student := range problem.students {
				v.reporter.Printf("  - %s", student)
			}
		}
	} else {
		v.reporter.StopProgress(aurora.Sprintf(aurora.Green("✓ keine Konflikte; %d Konflikte einzelner Studierender sind akzeptiert"),
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
		var startTime time.Time
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
	durByAncode map[int]examDurations, locByAncode map[int]string, validationMessages *map[conflictingAncodes]*problemWithStudents) {
	log.Debug().Str("name", student.Name).Str("mtknr", student.Mtknr).Msg("checking regs for student")

	planAncodeEntriesForStudent := make([]*model.PlanEntry, 0)
	for _, ancode := range student.ZpaAncodes {
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

	endOf := func(pe *model.PlanEntry) time.Time {
		if pe.Starttime == nil {
			return time.Time{}
		}
		return pe.Starttime.Add(time.Duration(durByAncode[pe.Ancode].forStudent(student.Mtknr)) * time.Minute)
	}

	p := planAncodeEntriesForStudent
	for i := 0; i < len(planAncodeEntriesForStudent); i++ {
		for j := i + 1; j < len(planAncodeEntriesForStudent); j++ {
			if p[i].Ancode == p[j].Ancode {
				continue
			}
			if p[i].Starttime == nil || p[j].Starttime == nil {
				continue // an unplaced / timeless entry cannot conflict
			}
			// A conflict where only ONE side is foreign is ours to fix — we can move our own
			// exam away from the other faculty's fixed one — so it is always reported. Only a
			// BOTH-foreign conflict is not ours to resolve; onlyPlannedByMe drops those (and is
			// the only thing that flag now does — it must never hide a conflict touching one of
			// our own exams).
			iForeign := foreignAncodes.Contains(p[i].Ancode)
			jForeign := foreignAncodes.Contains(p[j].Ancode)
			if iForeign && jForeign && onlyPlannedByMe {
				log.Debug().Int("ancode1", p[i].Ancode).Int("ancode2", p[j].Ancode).
					Msg("both ancodes not planned by me (foreign) — dropped in onlyPlannedByMe mode")
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
			gap := effectiveGapMinutes(plexams.semesterConfig.ExamGapMinutes, plexams.crossCampusGapMinutes(), locByAncode[p[i].Ancode], locByAncode[p[j].Ancode])
			switch _, label := conflictcalc.TimeProximity(
				*p[i].Starttime, endOf(p[i]), *p[j].Starttime, endOf(p[j]),
				gap, plexams.semesterConfig.NotTooCloseMinutes); label {
			case conflictcalc.Overlap:
				problem = conflictOverlap
			case conflictcalc.TooClose:
				problem = conflictTooClose
			case conflictcalc.SameDay:
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
				studentLine := fmt.Sprintf("%s (%s %s)", student.Name, student.Program, student.Group)
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
	v := newValidation(reporter, "constraints", "prüfe die Vorgaben der Prüfungen")

	if ok, err := p.planGenerated(ctx); err != nil {
		return nil, err
	} else if !ok {
		return v.skip(skipNoPlan), nil
	}

	v.step("lese die Vorgaben")
	constraints, err := p.Constraints(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get constraints")
	}

	constraintsMap := make(map[int]*model.Constraints)
	for _, constraint := range constraints {
		constraintsMap[constraint.Ancode] = constraint
	}

	v.step("lese die Anny-Buchungen")
	annyRoomBookings, err := p.ExahmRoomsFromAnnyBookings(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get entries from anny_bookings")
		return nil, err
	}

	examNames := p.newExamNamer(ctx, nil)
	for _, constraint := range constraints {
		v.step("prüfe die Vorgaben von Ancode %d", constraint.Ancode)
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

				v.errorf(ref{Ancode: ptr(constraint.Ancode), RelatedAncodes: []int{otherAncode}, Starttime: &slot.Starttime}.hint("Im Prüfungsplan die zweite Prüfung auf denselben Termin legen."),
					"%s soll zur selben Zeit liegen wie %s. Die erste liegt am %s, die zweite hat keinen Termin.",
					examNames.name(constraint.Ancode), examNames.name(otherAncode), slotLabelDE(slot.Starttime))
				continue
			}

			if !slot.Starttime.Equal(otherSlot.Starttime) {
				v.errorf(ref{Ancode: ptr(constraint.Ancode), RelatedAncodes: []int{otherAncode}, Starttime: &slot.Starttime}.hint("Im Prüfungsplan beide Prüfungen auf denselben Termin legen, oder unter „Prüfungen auswählen & Vorgaben“ die Vorgabe „zur selben Zeit wie“ entfernen."),
					"%s soll zur selben Zeit liegen wie %s, liegt aber am %s und die andere am %s.",
					examNames.name(constraint.Ancode), examNames.name(otherAncode), slotLabelDE(slot.Starttime), slotLabelDE(otherSlot.Starttime))
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
				v.errorf(ref{Ancode: ptr(constraint.Ancode), Starttime: &slot.Starttime}.hint("Im Prüfungsplan die Prüfung auf den festen Termin legen."),
					"%s hat den festen Termin %s, liegt aber am %s.", examNames.name(constraint.Ancode), slotLabelDE(*fixed), slotLabelDE(slot.Starttime))
			}
		}

		for _, day := range constraint.ExcludeDays {
			if day.Equal(time.Date(slot.Starttime.Year(), slot.Starttime.Month(), slot.Starttime.Day(), 0, 0, 0, 0, time.Local)) {
				v.errorf(ref{Ancode: ptr(constraint.Ancode), Starttime: &slot.Starttime}.hint("Im Prüfungsplan die Prüfung auf einen anderen Tag legen."),
					"%s liegt am %s, an diesem Tag soll sie aber nicht stattfinden (Sperrtag der Prüfung).", examNames.name(constraint.Ancode), slotLabelDE(slot.Starttime))
			}
		}

		if len(constraint.PossibleDays) > 0 {
			plannedDay := time.Date(slot.Starttime.Year(), slot.Starttime.Month(), slot.Starttime.Day(), 0, 0, 0, 0, time.Local)
			possible := make([]string, 0, len(constraint.PossibleDays))
			possibleDaysOk := false
			for _, day := range constraint.PossibleDays {
				if day.Equal(plannedDay) {
					possibleDaysOk = true
				}
				possible = append(possible, weekdaysDE[int(day.Weekday())]+" "+day.Format("02.01."))
			}
			if !possibleDaysOk {
				v.errorf(ref{Ancode: ptr(constraint.Ancode), Starttime: &slot.Starttime}.hint("Im Prüfungsplan die Prüfung auf einen der möglichen Tage legen."),
					"%s liegt am %s, darf aber nur an diesen Tagen stattfinden: %s.",
					examNames.name(constraint.Ancode), slotLabelDE(slot.Starttime), strings.Join(possible, ", "))
			}
		}

		if constraint.RoomConstraints != nil && (constraint.RoomConstraints.Exahm || constraint.RoomConstraints.Seb) {
			if !anny.RoomBookedDuringExamTime(annyRoomBookings, slot) {
				v.errorf(ref{Ancode: ptr(constraint.Ancode), Starttime: &slot.Starttime}.hint("Die Prüfung im Prüfungsplan in eine Zeit legen, für die ein Raum im T-Bau gebucht ist, oder für diese Zeit einen Raum in Anny buchen und die Buchungen neu laden."),
					"%s ist eine Rechnerprüfung (EXaHM/SEB) und liegt am %s, aber zu dieser Zeit ist in Anny kein passender Raum gebucht.",
					examNames.name(constraint.Ancode), slotLabelDE(slot.Starttime))
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
					v.errorf(ref{Ancode: ptr(constraint.Ancode), Room: ptr(room.RoomName), Starttime: &slot.Starttime}.hint("Im Raumplan einen der erlaubten Räume zuweisen, oder unter „Prüfungen auswählen & Vorgaben“ die erlaubten Räume erweitern."),
						"%s sitzt in Raum %s, erlaubt sind aber nur: %s.", examNames.name(constraint.Ancode), room.RoomName, strings.Join(constraint.RoomConstraints.AllowedRooms, ", "))
				}
			}
			if len(plannedRooms) == 0 {
				v.errorf(ref{Ancode: ptr(constraint.Ancode), Starttime: &slot.Starttime}.hint("Die Räume generieren oder der Prüfung im Raumplan einen der erlaubten Räume geben."),
					"%s hat noch keinen Raum. Erlaubt sind nur: %s.", examNames.name(constraint.Ancode), strings.Join(constraint.RoomConstraints.AllowedRooms, ", "))
			}
		}
	}

	return v.finish(), nil
}
