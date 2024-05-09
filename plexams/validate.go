package plexams

import (
	"context"
	"fmt"
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

	validationMessages := make([]string, 0)

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
		validateStudentReg(student, planAncodeEntries, planAncodeEntriesNotPlannedByMe, onlyPlannedByMe,
			knownConflicts, ancode, &validationMessages)
	}

	if len(validationMessages) > 0 {
		spinner.StopFailMessage(aurora.Sprintf(aurora.Red("%d known conflicts, but %d problems found"),
			knownConflictsCount, len(validationMessages)))
		err = spinner.StopFail()
		if err != nil {
			log.Debug().Err(err).Msg("cannot stop spinner")
		}
		for _, msg := range validationMessages {
			fmt.Printf("%s\n", msg)
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

func validateStudentReg(student *model.Student, planAncodeEntries []*model.PlanEntry,
	planAncodeEntriesNotPlannedByMe set.Set[int], onlyPlannedByMe bool, knownConflicts set.Set[KnownConflict], ancode int,
	validationMessages *[]string) {
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
				// fmt.Printf("found known conflict: [%s, %d, %d]\n", student.Mtknr, p[i].Ancode, p[j].Ancode)
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

			// same slot
			if p[i].DayNumber == p[j].DayNumber &&
				p[i].SlotNumber == p[j].SlotNumber {
				count++
				*validationMessages = append(*validationMessages, aurora.Sprintf(aurora.Red("    - [\"%s\", \"%d\", \"%d\"] # %3d. Same slot: ancodes %d (%d, %d) and %d (%d,%d) for student %s (%s/%s)"),
					aurora.Magenta(student.Mtknr), aurora.Magenta(p[i].Ancode), aurora.Magenta(p[j].Ancode), count,
					p[i].Ancode, p[i].DayNumber, p[i].SlotNumber,
					p[j].Ancode, p[j].DayNumber, p[j].SlotNumber,
					aurora.Cyan(student.Name), aurora.Cyan(student.Program), aurora.Cyan(student.Mtknr),
				))
			} else
			// adjacent slots
			if p[i].DayNumber == p[j].DayNumber &&
				(p[i].SlotNumber+1 == p[j].SlotNumber ||
					p[i].SlotNumber-1 == p[j].SlotNumber) {
				count++
				*validationMessages = append(*validationMessages, aurora.Sprintf(aurora.Red("    - [\"%s\", \"%d\", \"%d\"] # %3d. Adjacent slots: ancodes %d (%d, %d) and %d (%d,%d) for student %s (%s/%s)"),
					aurora.Magenta(student.Mtknr), aurora.Magenta(p[i].Ancode), aurora.Magenta(p[j].Ancode), count,
					p[i].Ancode, p[i].DayNumber, p[i].SlotNumber,
					p[j].Ancode, p[j].DayNumber, p[j].SlotNumber,
					aurora.Cyan(student.Name), aurora.Cyan(student.Program), aurora.Cyan(student.Mtknr),
				))
			} else
			// same day
			if p[i].DayNumber == p[j].DayNumber {
				count++
				*validationMessages = append(*validationMessages, aurora.Sprintf(aurora.Yellow("    - [\"%s\", \"%d\", \"%d\"] # %3d. Same day: ancodes %d (%d, %d) and %d (%d,%d) for student %s (%s/%s)"),
					aurora.Magenta(student.Mtknr), aurora.Magenta(p[i].Ancode), aurora.Magenta(p[j].Ancode), count,
					p[i].Ancode, p[i].DayNumber, p[i].SlotNumber,
					p[j].Ancode, p[j].DayNumber, p[j].SlotNumber,
					aurora.Cyan(student.Name), aurora.Cyan(student.Program), aurora.Cyan(student.Mtknr),
				))
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
	bookedEntries, err := p.ExahmRoomsFromBooked()
	if err != nil {
		log.Error().Err(err).Msg("cannot get booked entries")
		return err
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
						aurora.Magenta(constraint.Ancode), aurora.Cyan(day.Format("02.01.06"))))
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

		if constraint.RoomConstraints != nil && constraint.RoomConstraints.ExahmRooms || constraint.RoomConstraints.Seb {
			if !p.roomBookedDuringExamTime(bookedEntries, slot) {
				validationMessages = append(validationMessages,
					aurora.Sprintf(aurora.Red("Exam %d planned at %s, but no room booked"),
						aurora.Magenta(constraint.Ancode), aurora.Cyan(slot.Starttime.Format("02.01.06 15:04"))))
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
	for _, bookedEntry := range bookedEntries {
		if bookedEntry.From.Before(slot.Starttime) && bookedEntry.Until.After(slot.Starttime.Add(90*time.Minute)) {
			return true
		}
	}

	return false
}
