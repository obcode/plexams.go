package plexams

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) AddExamGroupToSlot(ctx context.Context, dayNumber int, timeNumber int, examGroupCode int) (bool, error) {
	// check if slot exists
	ok := false
	for _, day := range p.semesterConfig.Days {
		if day.Number == dayNumber {
			ok = true
			break
		}
	}
	if !ok {
		log.Error().Int("day", dayNumber).Msg("day does not exists")
		return false, fmt.Errorf("day %d does not exist", dayNumber)
	}
	ok = false
	for _, time := range p.semesterConfig.Starttimes {
		if time.Number == timeNumber {
			ok = true
			break
		}
	}
	if !ok {
		log.Error().Int("time", timeNumber).Msg("time does not exists")
		return false, fmt.Errorf("time %d does not exist", timeNumber)
	}

	// check if examGroup exists
	_, err := p.ExamGroup(ctx, examGroupCode)
	if err != nil {
		log.Error().Err(err).Int("examGroupCode", examGroupCode).Msg("exam group does not exist")
	}

	// check if slot is allowed
	allowedSlots, err := p.AllowedSlots(ctx, examGroupCode)
	if err != nil {
		log.Error().Err(err).Int("examGroupCode", examGroupCode).Msg("cannot get allowed slots")
	}
	slotIsAllowed := false

	for _, slot := range allowedSlots {
		if slot.DayNumber == dayNumber && slot.SlotNumber == timeNumber {
			slotIsAllowed = true
			break
		}
	}
	if !slotIsAllowed {
		log.Debug().Int("day", dayNumber).Int("time", timeNumber).Int("examGroupCode", examGroupCode).
			Msg("slot is not allowed")
		return false, fmt.Errorf("slot (%d,%d) is not allowed for exam group %d",
			dayNumber, timeNumber, examGroupCode)
	}

	return p.dbClient.AddExamGroupToSlot(ctx, dayNumber, timeNumber, examGroupCode)
}

func (p *Plexams) ExamGroupsInSlot(ctx context.Context, day int, time int) ([]*model.ExamGroup, error) {
	return p.dbClient.ExamGroupsInSlot(ctx, day, time)
}

func (p *Plexams) AllowedSlots(ctx context.Context, examGroupCode int) ([]*model.Slot, error) {
	examGroup, err := p.ExamGroup(ctx, examGroupCode)
	if err != nil {
		log.Error().Err(err).Int("examGroupCode", examGroupCode).Msg("exam group does not exist")
	}

	allowedSlots := make([]*model.Slot, 0)
OUTER:
	for _, slot := range examGroup.ExamGroupInfo.PossibleSlots {
		// get ExamGroups for slot and check Conflicts
		examGroups, err := p.ExamGroupsInSlot(ctx, slot.DayNumber, slot.SlotNumber)
		if err != nil {
			log.Error().Err(err).Int("day", slot.DayNumber).Int("time", slot.SlotNumber).
				Msg("cannot get exam groups in slot")
			return nil, err
		}
		for _, otherExamGroup := range examGroups {
			for _, conflict := range examGroup.ExamGroupInfo.Conflicts {
				if otherExamGroup.ExamGroupCode == conflict.ExamGroupCode {
					continue OUTER
				}
			}
		}
		allowedSlots = append(allowedSlots, slot)
	}

	return allowedSlots, nil
}
