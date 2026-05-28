package plexams

import (
	"context"
	"fmt"
	"sort"
	"time"

	set "github.com/deckarep/golang-set/v2"
	"github.com/gookit/color"
	"github.com/logrusorgru/aurora"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"github.com/theckman/yacspin"
)

// TODO: Validate if all NTAs have MTKNR

var (
	count               = 0
	knownConflictsCount = 0
)

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

func (p *Plexams) ValidateConflicts(onlyPlannedByMe bool, ancode int) error {
	count = 0
	knownConflictsCount = 0
	ctx := context.Background()
	cfg := yacspin.Config{
		Frequency:         100 * time.Millisecond,
		CharSet:           yacspin.CharSets[69],
		Suffix:            aurora.Sprintf(aurora.Cyan(" validating conflicts")),
		SuffixAutoColon:   true,
		StopCharacter:     "✓",
		StopColors:        []string{"fgGreen"},
		StopFailMessage:   "error",
		StopFailCharacter: "✗",
		StopFailColors:    []string{"fgRed"},
	}

	spinner, err := yacspin.New(cfg)
	if err != nil {
		log.Debug().Err(err).Msg("cannot create spinner")
	}
	err = spinner.Start()
	if err != nil {
		log.Debug().Err(err).Msg("cannot start spinner")
	}

	validationMessages := make(map[conflictingAncodes]*problemWithStudents)

	spinner.Message(aurora.Sprintf(aurora.Yellow(" get planned ancodes")))
	planAncodeEntries, err := p.dbClient.PlannedAncodes(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get plan entries")
		return err
	}

	planAncodeEntriesNotPlannedByMe := set.NewSet[int]()
	for _, entry := range planAncodeEntries {
		constraints, err := p.dbClient.GetConstraintsForAncode(ctx, entry.Ancode)
		if err != nil {
			log.Error().Err(err).Int("ancode", entry.Ancode).Msg("cannot get constraints for ancode")
			return err
		}
		if constraints != nil && constraints.NotPlannedByMe {
			planAncodeEntriesNotPlannedByMe.Add(entry.Ancode)
		}
	}

	spinner.Message(aurora.Sprintf(aurora.Yellow(" get student regs")))
	students, err := p.StudentRegsPerStudentPlanned(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get student registries per student")
		return err
	}

	knownConflicts := set.NewSet[KnownConflict]()

	spinner.Message(aurora.Sprintf(aurora.Yellow(" get known conflicts")))
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

	spinner.Message(aurora.Sprintf(aurora.Yellow(" validating students")))
	for _, student := range students {
		p.validateStudentReg(student, planAncodeEntries, planAncodeEntriesNotPlannedByMe, onlyPlannedByMe,
			knownConflicts, ancode, &validationMessages)
	}

	conflictingAncodesSlice, normalizedValidationMessages := p.sortConflictingAncodes(validationMessages)

	if len(validationMessages) > 0 {
		mucdaiPrograms := viper.GetStringSlice("mucdaiprograms")
		mucdaiprogram := make(map[int]string)
		for _, p := range mucdaiPrograms {
			base := viper.GetInt(fmt.Sprintf("externalExamsBase.%s", p))
			mucdaiprogram[base] = p
		}

		spinner.StopFailMessage(aurora.Sprintf(aurora.Red("%d known conflicts, but %d problems found"),
			knownConflictsCount, len(validationMessages)))
		err = spinner.StopFail()
		if err != nil {
			log.Debug().Err(err).Msg("cannot stop spinner")
		}
		fmt.Printf("\nknownConflicts:\n  studentRegs:")
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

			log.Debug().Interface("exam1", exam1).Interface("exam2", exam2).Msg("found conflicting exams")
			fmt.Printf("%s\n", aurora.Sprintf(aurora.Red("\n    # %s"), problemWithStudents.problem))
			for _, exam := range []*model.PlannedExam{exam1, exam2} {
				repeater := ""
				if exam.ZpaExam.IsRepeaterExam {
					repeater = "-- Wiederholungsprüfung"
				}
				ancode := exam.Ancode
				ancodeStr := fmt.Sprintf("%6d", ancode)
				zpaAncode := "           "
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

				planEntry := exam.PlanEntry
				time := p.getSlotTime(planEntry.DayNumber, planEntry.SlotNumber)
				if planEntry.ExternalTime != nil {
					time = *planEntry.ExternalTime
				}

				fmt.Printf("%s\n", aurora.Sprintf(aurora.Red("    # %s - %s. %s (%s): %s %s %s"),
					time.Local().Format("02.01.06, 15:04 Uhr"),
					ancodeStr,
					aurora.Cyan(exam.ZpaExam.Module), aurora.Cyan(exam.ZpaExam.MainExamer),
					aurora.Yellow(exam.ZpaExam.Groups),
					aurora.Cyan(repeater),
					zpaAncode,
				))
			}
			for _, studentStr := range problemWithStudents.students {
				fmt.Printf("%s\n", studentStr)
			}
		}

	} else {
		spinner.StopMessage(aurora.Sprintf(aurora.Green("%d known conflicts, no further problems found"),
			knownConflictsCount))
		err = spinner.Stop()
		if err != nil {
			log.Debug().Err(err).Msg("cannot stop spinner")
		}
	}

	return nil
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
				problem = "same slot"
			} else
			// adjacent slots
			if p[i].DayNumber == p[j].DayNumber &&
				(p[i].SlotNumber+1 == p[j].SlotNumber ||
					p[i].SlotNumber-1 == p[j].SlotNumber) {
				problem = "adjacent slot"
			} else
			// same day
			if p[i].DayNumber == p[j].DayNumber {
				problem = "same day"
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

func (p *Plexams) ValidateConstraints() error {
	count = 0
	ctx := context.Background()
	cfg := yacspin.Config{
		Frequency:         100 * time.Millisecond,
		CharSet:           yacspin.CharSets[69],
		Suffix:            aurora.Sprintf(aurora.Cyan(" validating constraints")),
		SuffixAutoColon:   true,
		StopCharacter:     "✓",
		StopColors:        []string{"fgGreen"},
		StopFailMessage:   "error",
		StopFailCharacter: "✗",
		StopFailColors:    []string{"fgRed"},
	}

	spinner, err := yacspin.New(cfg)
	if err != nil {
		log.Debug().Err(err).Msg("cannot create spinner")
	}
	err = spinner.Start()
	if err != nil {
		log.Debug().Err(err).Msg("cannot start spinner")
	}

	validationMessages := make([]string, 0)

	spinner.Message(aurora.Sprintf(aurora.Yellow(" get constraints")))
	constraints, err := p.Constraints(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get constraints")
	}

	constraintsMap := make(map[int]*model.Constraints)
	for _, constraint := range constraints {
		constraintsMap[constraint.Ancode] = constraint
	}

	spinner.Message(aurora.Sprintf(aurora.Yellow(" get booked entries")))
	bookedEntries, err := p.ExahmRoomsFromAnnyBookings(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get entries from anny_bookings, fallback to booked entries in YAML")
		bookedEntries = nil
	}
	if len(bookedEntries) == 0 {
		bookedEntries, err = p.ExahmRoomsFromBooked()
		if err != nil {
			log.Error().Err(err).Msg("cannot get booked entries")
			return err
		}
	}

	for _, constraint := range constraints {
		count++
		spinner.Message(aurora.Sprintf(aurora.Yellow(" check constraints for exam %d"), aurora.Magenta(constraint.Ancode)))
		slot, err := p.SlotForAncode(ctx, constraint.Ancode)
		if err != nil {
			log.Error().Err(err).Int("ancode", constraint.Ancode).Msg("cannot get slot for ancode")
		}

		if slot == nil {
			continue
		}

		// if len(constraint.SameSlot) > 0 {
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

				validationMessages = append(validationMessages,
					aurora.Sprintf(aurora.Red("Exams %d and %d must be in the same slot, are %v and %v"),
						aurora.Magenta(constraint.Ancode), aurora.Magenta(otherAncode), aurora.Cyan(slot), aurora.Cyan(otherSlot)))
				continue
			}

			if *slot != *otherSlot {
				validationMessages = append(validationMessages,
					aurora.Sprintf(aurora.Red("Exams %d and %d must be in the same slot, are %v and %v"),
						aurora.Magenta(constraint.Ancode), aurora.Magenta(otherAncode), aurora.Cyan(slot), aurora.Cyan(otherSlot)))
			}
		}
		// }

		if constraint.FixedDay != nil {
			color.Red.Println("FIXME: FixedDay")
		}

		if constraint.FixedTime != nil {
			log.Debug().Int("ancode", constraint.Ancode).Msg("checking fixed time")
			fixed := constraint.FixedTime
			if fixed.Day() != slot.Starttime.Day() ||
				fixed.Month() != slot.Starttime.Month() ||
				fixed.Local().Hour() != slot.Starttime.Local().Hour() ||
				fixed.Minute() != slot.Starttime.Minute() {
				validationMessages = append(validationMessages,
					aurora.Sprintf(aurora.Red("Exams %d has fixed slot %s, is %s"),
						aurora.Magenta(constraint.Ancode), aurora.Magenta(fixed.Format("02.01.06 15:04")), aurora.Cyan(constraint.FixedTime.Format("02.01.06 15:04"))))
			}
		}

		for _, day := range constraint.ExcludeDays {
			if day.Equal(time.Date(slot.Starttime.Year(), slot.Starttime.Month(), slot.Starttime.Day(), 0, 0, 0, 0, time.Local)) {
				validationMessages = append(validationMessages,
					aurora.Sprintf(aurora.Red("Exam %d planned on excluded day %s"),
						aurora.Magenta(constraint.Ancode), aurora.Cyan(day.Local().Format("02.01.06"))))
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
				validationMessages = append(validationMessages,
					aurora.Sprintf(aurora.Red("Exam %d planned on day %s which is not a possible day"),
						aurora.Magenta(constraint.Ancode), aurora.Cyan(dayPlanned.Format("02.01.06"))))
			}
		}

		if constraint.RoomConstraints != nil && (constraint.RoomConstraints.Exahm || constraint.RoomConstraints.Seb) {
			if !p.roomBookedDuringExamTime(bookedEntries, slot) {
				validationMessages = append(validationMessages,
					aurora.Sprintf(aurora.Red("Exam %d planned at %s, but no room booked"),
						aurora.Magenta(constraint.Ancode), aurora.Cyan(slot.Starttime.Format("02.01.06 15:04"))))
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
					validationMessages = append(validationMessages,
						aurora.Sprintf(aurora.Red("Exam %d planned in room %s, but allowed rooms are %s"),
							aurora.Magenta(constraint.Ancode), aurora.Cyan(room.RoomName), aurora.Cyan(constraint.RoomConstraints.AllowedRooms)))
				}
			}
			if len(plannedRooms) == 0 {
				validationMessages = append(validationMessages,
					aurora.Sprintf(aurora.Red("Exam %d planned in no room, but allowed rooms are %s"),
						aurora.Magenta(constraint.Ancode), aurora.Cyan(constraint.RoomConstraints.AllowedRooms)))
			}
		}
	}

	if len(validationMessages) > 0 {
		spinner.StopFailMessage(aurora.Sprintf(aurora.Red("%d known constraints, but %d problems found"),
			count, len(validationMessages)))
		err = spinner.StopFail()
		if err != nil {
			log.Debug().Err(err).Msg("cannot stop spinner")
		}
		for _, msg := range validationMessages {
			fmt.Printf("%s\n", msg)
		}

	} else {
		spinner.StopMessage(aurora.Sprintf(aurora.Green("%d known constraints, no problems found"),
			count))
		err = spinner.Stop()
		if err != nil {
			log.Debug().Err(err).Msg("cannot stop spinner")
		}
	}

	return nil
}

func (p *Plexams) roomBookedDuringExamTime(bookedEntries []BookedEntry, slot *model.Slot) bool {
	if slot == nil {
		return false
	}

	examStart := slot.Starttime
	examEnd := slot.Starttime.Add(90 * time.Minute)

	for _, bookedEntry := range bookedEntries {
		// Inclusive overlap: bookings starting/ending exactly at exam boundaries are valid.
		if !bookedEntry.From.After(examStart) && !bookedEntry.Until.Before(examEnd) {
			return true
		}
	}

	return false
}
