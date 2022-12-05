package plexams

import (
	"context"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) PlannedExamsForProgram(ctx context.Context, program string) ([]*model.PlannedExam, error) {
	connectedExams, err := p.GetConnectedExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get connected exams")
		return nil, err
	}

	plannedExams := make([]*model.PlannedExam, 0)

	for _, connectedExam := range connectedExams {
		constraints, _ := p.ConstraintForAncode(ctx, connectedExam.ZpaExam.AnCode)
		if constraints != nil && constraints.NotPlannedByMe {
			// if notPlannedByMe, _ := p.NotPlannedByMe(ctx, connectedExam.ZpaExam.AnCode); notPlannedByMe {
			log.Debug().Int("ancode", connectedExam.ZpaExam.AnCode).
				Str("module", connectedExam.ZpaExam.Module).
				Str("main examer", connectedExam.ZpaExam.MainExamer).
				Msg("exam not planned by me")
			continue
		}

		var plannedExam *model.PlannedExam
		for _, primussExam := range connectedExam.PrimussExams {
			if primussExam.Program == program {
				plannedExam = &model.PlannedExam{
					Ancode:     connectedExam.ZpaExam.AnCode,
					Module:     connectedExam.ZpaExam.Module,
					MainExamer: connectedExam.ZpaExam.MainExamer,
					DateTime:   nil,
				}
			}
		}

		if plannedExam == nil {
			continue
		}

		log.Debug().Int("ancode", plannedExam.Ancode).Msg("found connected exam")

		slot, err := p.SlotForAncode(ctx, plannedExam.Ancode)
		if err != nil {
			log.Error().Err(err).Int("ancode", plannedExam.Ancode).Msg("cannot get slot for ancode")
			return nil, err
		}
		if slot != nil {
			plannedExam.DateTime = p.getTimeForSlot(slot.DayNumber, slot.SlotNumber)
		}
		plannedExams = append(plannedExams, plannedExam)
	}

	return plannedExams, nil
}

func (p *Plexams) getTimeForSlot(dayNumber, slotNumber int) *time.Time {
	for _, slot := range p.semesterConfig.Slots {
		if slot.DayNumber == dayNumber && slot.SlotNumber == slotNumber {
			return &slot.Starttime
		}
	}
	return nil
}
