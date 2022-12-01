package plexams

import (
	"context"
	"time"

	"github.com/gookit/color"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) ValidateConflicts() error {
	ctx := context.Background()
	color.Style{color.FgRed, color.BgGreen, color.OpBold}.Println(" ---   validating conflicts   --- ")

	planAncodeEntries, err := p.dbClient.PlanAncodeEntries(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get plan entries")
		return err
	}

	studentRegs, err := p.StudentRegsPerStudentPlanned(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get student registries per student")
		return err
	}

	for _, studentReg := range studentRegs {
		validateStudentReg(studentReg, planAncodeEntries)
	}
	return nil
}

func validateStudentReg(studentReg *model.StudentRegsPerStudent, planAncodeEntries []*model.PlanAncodeEntry) {
	log.Debug().Str("name", studentReg.Student.Name).Str("mtknr", studentReg.Student.Mtknr).Msg("checking regs for student")

	planAncodeEntriesForStudent := make([]*model.PlanAncodeEntry, 0)
	for _, ancode := range studentReg.Ancodes {
		for _, planEntry := range planAncodeEntries {
			if ancode == planEntry.Ancode {
				planAncodeEntriesForStudent = append(planAncodeEntriesForStudent, planEntry)
			}
		}
	}

	if len(planAncodeEntriesForStudent) == 0 {
		log.Debug().Str("name", studentReg.Student.Name).Str("mtknr", studentReg.Student.Mtknr).Msg("no exam for student in plan")
		return
	}

	p := planAncodeEntriesForStudent
	for i := 0; i < len(planAncodeEntriesForStudent); i++ {
		for j := i + 1; j < len(planAncodeEntriesForStudent); j++ {
			if p[i].DayNumber == p[j].DayNumber &&
				p[i].SlotNumber == p[j].SlotNumber &&
				p[i].Ancode == p[j].Ancode {
				continue
			}
			// same slot
			if p[i].DayNumber == p[j].DayNumber &&
				p[i].SlotNumber == p[j].SlotNumber {
				color.Red.Printf("Same slot: ancodes %d (%d, %d) and %d (%d,%d) for student %s (%s/%s)\n",
					p[i].Ancode, p[i].DayNumber, p[i].SlotNumber,
					p[j].Ancode, p[j].DayNumber, p[j].SlotNumber,
					studentReg.Student.Name, studentReg.Student.Program, studentReg.Student.Mtknr,
				)
			} else
			// adjacent slots
			if p[i].DayNumber == p[j].DayNumber &&
				(p[i].SlotNumber+1 == p[j].SlotNumber ||
					p[i].SlotNumber-1 == p[j].SlotNumber) {
				color.Red.Printf("Adjacent slots: ancodes %d (%d, %d) and %d (%d,%d) for student %s (%s/%s)\n",
					p[i].Ancode, p[i].DayNumber, p[i].SlotNumber,
					p[j].Ancode, p[j].DayNumber, p[j].SlotNumber,
					studentReg.Student.Name, studentReg.Student.Program, studentReg.Student.Mtknr,
				)
			} else
			// same day
			if p[i].DayNumber == p[j].DayNumber {
				color.Yellow.Printf("Same day: ancodes %d (%d, %d) and %d (%d,%d) for student %s (%s/%s)\n",
					p[i].Ancode, p[i].DayNumber, p[i].SlotNumber,
					p[j].Ancode, p[j].DayNumber, p[j].SlotNumber,
					studentReg.Student.Name, studentReg.Student.Program, studentReg.Student.Mtknr,
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
