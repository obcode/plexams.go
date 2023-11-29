package plexams

import (
	"context"
	"time"

	set "github.com/deckarep/golang-set/v2"
	"github.com/gookit/color"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// TODO: Validate if all NTAs have MTKNR

var count = 0

func (p *Plexams) ValidateConflicts(onlyPlannedByMe bool, ancode int) error {
	count = 0
	ctx := context.Background()
	color.Style{color.FgRed, color.BgGreen, color.OpBold}.Println(" ---   validating conflicts   --- ")

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

	students, err := p.StudentRegsPerStudentPlanned(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get student registries per student")
		return err
	}

	for _, student := range students {
		validateStudentReg(student, planAncodeEntries, planAncodeEntriesNotPlannedByMe, onlyPlannedByMe, ancode)
	}
	return nil
}

func validateStudentReg(student *model.Student, planAncodeEntries []*model.PlanAncodeEntry,
	planAncodeEntriesNotPlannedByMe set.Set[int], onlyPlannedByMe bool, ancode int) {
	log.Debug().Str("name", student.Name).Str("mtknr", student.Mtknr).Msg("checking regs for student")

	planAncodeEntriesForStudent := make([]*model.PlanAncodeEntry, 0)
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
				color.Red.Printf("%3d. Same slot: ancodes %d (%d, %d) and %d (%d,%d) for student %s (%s/%s)\n", count,
					p[i].Ancode, p[i].DayNumber, p[i].SlotNumber,
					p[j].Ancode, p[j].DayNumber, p[j].SlotNumber,
					student.Name, student.Program, student.Mtknr,
				)
			} else
			// adjacent slots
			if p[i].DayNumber == p[j].DayNumber &&
				(p[i].SlotNumber+1 == p[j].SlotNumber ||
					p[i].SlotNumber-1 == p[j].SlotNumber) {
				count++
				color.Red.Printf("%3d. Adjacent slots: ancodes %d (%d, %d) and %d (%d,%d) for student %s (%s/%s)\n", count,
					p[i].Ancode, p[i].DayNumber, p[i].SlotNumber,
					p[j].Ancode, p[j].DayNumber, p[j].SlotNumber,
					student.Name, student.Program, student.Mtknr,
				)
			} else
			// same day
			if p[i].DayNumber == p[j].DayNumber {
				count++
				color.Yellow.Printf("%3d. Same day: ancodes %d (%d, %d) and %d (%d,%d) for student %s (%s/%s)\n", count,
					p[i].Ancode, p[i].DayNumber, p[i].SlotNumber,
					p[j].Ancode, p[j].DayNumber, p[j].SlotNumber,
					student.Name, student.Program, student.Mtknr,
				)
			}
		}
	}
}

func (p *Plexams) ValidateConstraints() error {
	ctx := context.Background()
	color.Style{color.FgRed, color.BgGreen, color.OpBold}.Println(" ---   validating constraints   --- ")

	constraints, err := p.Constraints(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get constraints")
	}

	for _, constraint := range constraints {
		slot, err := p.SlotForAncode(ctx, constraint.Ancode)
		if err != nil {
			log.Error().Err(err).Int("ancode", constraint.Ancode).Msg("cannot get slot for ancode")
		}

		if slot == nil {
			continue
		}

		if constraint.FixedDay != nil {
			color.Red.Println("FIXME: FixedDay")
		}

		if constraint.FixedTime != nil {
			color.Red.Println("FIXME: FixedTime")
		}

		for _, day := range constraint.ExcludeDays {
			if day.Equal(time.Date(slot.Starttime.Year(), slot.Starttime.Month(), slot.Starttime.Day(), 0, 0, 0, 0, time.Local)) {
				color.Red.Printf("Exam #%d planned on excluded day %s\n", constraint.Ancode, day.Format("02.01.06"))
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
				color.Red.Printf("Exam #%d planned on day %s which is not a possible day\n", constraint.Ancode, dayPlanned.Format("02.01.06"))
			}
		}
	}

	return nil
}
