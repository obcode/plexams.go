package plexams

import (
	"context"
	"fmt"
	"sort"
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

type KnownConflict struct {
	Mtknr, Ancode1, Ancode2 string
}

type conflictingAncodes struct {
	ancode1 int
	ancode2 int
}
type problemWithStudents struct {
	problem  string
	students []string
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

func (p *Plexams) ValidateConflicts(onlyPlannedByMe bool, ancode int, reporter Reporter) (*model.ValidationReport, error) {
	knownConflictsCount = 0
	ctx := context.Background()
	v := newValidation(reporter, "conflicts", "validating conflicts")

	validationMessages := make(map[conflictingAncodes]*problemWithStudents)

	v.step("get planned ancodes")
	planAncodeEntries, err := p.dbClient.PlannedAncodes(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get plan entries")
		return nil, err
	}

	planAncodeEntriesNotPlannedByMe := set.NewSet[int]()
	for _, entry := range planAncodeEntries {
		constraints, err := p.dbClient.GetConstraintsForAncode(ctx, entry.Ancode)
		if err != nil {
			log.Error().Err(err).Int("ancode", entry.Ancode).Msg("cannot get constraints for ancode")
			return nil, err
		}
		if constraints != nil && constraints.NotPlannedByMe {
			planAncodeEntriesNotPlannedByMe.Add(entry.Ancode)
		}
	}

	v.step("get student regs")
	students, err := p.StudentRegsPerStudentPlanned(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get student registries per student")
		return nil, err
	}

	knownConflicts := set.NewSet[KnownConflict]()

	v.step("get known conflicts")
	knownConflictsConf := viper.Get("knownConflicts.studentRegs")
	if knownConflictsConf != nil {
		knownConflictsSlice := knownConflictsConf.([]interface{})
		for _, knownConflict := range knownConflictsSlice {
			knownConflictSlice := knownConflict.([]interface{})
			knownConflicts.Add(KnownConflict{
				Mtknr:   knownConflictSlice[0].(string),
				Ancode1: knownConflictSlice[1].(string),
				Ancode2: knownConflictSlice[2].(string),
			})
		}
	}

	log.Debug().Int("count", knownConflicts.Cardinality()).Interface("conflicts", knownConflicts).Msg("found known conflicts")

	v.step("validating students")
	for _, student := range students {
		p.validateStudentReg(student, planAncodeEntries, planAncodeEntriesNotPlannedByMe, onlyPlannedByMe,
			knownConflicts, ancode, &validationMessages)
	}

	conflictingAncodesSlice, normalizedValidationMessages := p.sortConflictingAncodes(validationMessages)

	// One structured finding per conflicting exam pair, graded by severity: same slot is
	// a hard clash (error), adjacent slots are undesirable (warning), same day is usually
	// acceptable (info).
	for _, ca := range conflictingAncodesSlice {
		problem := normalizedValidationMessages[ca]
		r := ref{Ancode: ptr(ca.ancode1), RelatedAncodes: []int{ca.ancode2}}
		const format = "%s: %d student(s) affected between exam %d and %d"
		switch problem.problem {
		case conflictSameSlot:
			v.errorf(r, format, problem.problem, len(problem.students), ca.ancode1, ca.ancode2)
		case conflictAdjacent:
			v.warnf(r, format, problem.problem, len(problem.students), ca.ancode1, ca.ancode2)
		default: // conflictSameDay
			v.infof(r, format, problem.problem, len(problem.students), ca.ancode1, ca.ancode2)
		}
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
		summary := aurora.Sprintf(aurora.Yellow("%d known conflicts; %d error(s), %d warning(s), %d info(s)"),
			knownConflictsCount, errs, warns, infos)
		if errs > 0 {
			v.reporter.StopProgressFail(summary)
		} else {
			v.reporter.StopProgress(summary)
		}

		// Stream the copy-pasteable knownConflicts YAML snippet, like the CLI.
		v.reporter.Println("")
		v.reporter.Println("knownConflicts:")
		v.reporter.Println("  studentRegs:")
		for _, conflictingAncodes := range conflictingAncodesSlice {
			problemWithStudents := normalizedValidationMessages[conflictingAncodes]
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
				if planEntry.ExternalTime != nil {
					time = *planEntry.ExternalTime
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
		if planEntry.ExternalTime != nil {
			startTime = *planEntry.ExternalTime
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
	planAncodeEntriesNotPlannedByMe set.Set[int], onlyPlannedByMe bool, knownConflicts set.Set[KnownConflict], ancode int,
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
			if knownConflicts.Contains(KnownConflict{
				Mtknr:   student.Mtknr,
				Ancode1: fmt.Sprint(p[i].Ancode),
				Ancode2: fmt.Sprint(p[j].Ancode),
			}) {
				knownConflictsCount++
				continue
			}
			if onlyPlannedByMe &&
				planAncodeEntriesNotPlannedByMe.Contains(p[i].Ancode) &&
				planAncodeEntriesNotPlannedByMe.Contains(p[j].Ancode) {
				log.Debug().Int("ancode1", p[i].Ancode).Int("ancode2", p[j].Ancode).
					Msg("both ancodes not planned by me")
				continue
			}
			if ancode != 0 && p[i].Ancode != ancode && p[j].Ancode != ancode {
				continue
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
				validationMessageForProblem.students = append(validationMessageForProblem.students,
					aurora.Sprintf(aurora.Yellow("    - [\"%s\", \"%d\", \"%d\"] # %s (%s%s / %s)"),
						aurora.Magenta(student.Mtknr), aurora.Magenta(p[i].Ancode), aurora.Magenta(p[j].Ancode),
						aurora.Cyan(student.Name), aurora.Cyan(student.Program), aurora.Cyan(student.Group), aurora.Cyan(student.Mtknr),
					))
				(*validationMessages)[conflictingAncodes] = validationMessageForProblem
			}
		}
	}
}

func (p *Plexams) ValidateConstraints(reporter Reporter) (*model.ValidationReport, error) {
	ctx := context.Background()
	v := newValidation(reporter, "constraints", "validating constraints")

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
