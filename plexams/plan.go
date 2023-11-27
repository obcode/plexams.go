package plexams

import (
	"context"
	"fmt"
	"sort"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) AddExamToSlot(ctx context.Context, ancode int, dayNumber int, timeNumber int) (bool, error) {
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

	// check if exam with ancode exists
	exam, err := p.GeneratedExam(ctx, ancode)
	if err != nil || exam == nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("exam does not exist or does not need to be planned")
		return false, err
	}

	// TODO: check if slot is allowed
	// allowedSlots, err := p.AllowedSlots(ctx, ancode)
	// if err != nil {
	// 	log.Error().Err(err).Int("ancode", ancode).Msg("cannot get allowed slots")
	// }
	// slotIsAllowed := false

	// for _, slot := range allowedSlots {
	// 	if slot.DayNumber == dayNumber && slot.SlotNumber == timeNumber {
	// 		slotIsAllowed = true
	// 		break
	// 	}
	// }
	// if !slotIsAllowed {
	// 	log.Debug().Int("day", dayNumber).Int("time", timeNumber).Int("ancode", ancode).
	// 		Msg("slot is not allowed")
	// 	return false, fmt.Errorf("slot (%d,%d) is not allowed for exam group %d",
	// 		dayNumber, timeNumber, ancode)
	// }

	return p.dbClient.AddExamToSlot(ctx, dayNumber, timeNumber, ancode)
}

func (p *Plexams) AllowedSlots(ctx context.Context, ancode int) ([]*model.Slot, error) {
	return nil, nil

	// 	if p.dbClient.ExamIsLocked(ctx, ancode) {
	// 		return []*model.Slot{}, nil
	// 	}
	// 	exam, err := p.GeneratedExam(ctx, ancode)
	// 	if err != nil {
	// 		log.Error().Err(err).Int("ancode", ancode).Msg("exam does not exist")
	// 	}

	// 	allowedSlots := make([]*model.Slot, 0)
	// OUTER:
	// 	for _, slot := range exam.Constraints.PossibleSlots {
	// 		// get ExamGroups for slot and check Conflicts
	// 		examGroups, err := p.ExamGroupsInSlot(ctx, slot.DayNumber, slot.SlotNumber)
	// 		if err != nil {
	// 			log.Error().Err(err).Int("day", slot.DayNumber).Int("time", slot.SlotNumber).
	// 				Msg("cannot get exam groups in slot")
	// 			return nil, err
	// 		}
	// 		for _, otherExamGroup := range examGroups {
	// 			for _, conflict := range examGroup.ExamGroupInfo.Conflicts {
	// 				if otherExamGroup.ExamGroupCode == conflict.ExamGroupCode {
	// 					continue OUTER
	// 				}
	// 			}
	// 		}

	// 		allowedSlots = append(allowedSlots, &model.Slot{
	// 			DayNumber:  slot.DayNumber,
	// 			SlotNumber: slot.SlotNumber,
	// 			Starttime:  p.getSlotTime(slot.DayNumber, slot.SlotNumber),
	// 		})
	// 	}

	// return allowedSlots, nil
}

func (p *Plexams) ExamsWithoutSlot(ctx context.Context) ([]*model.GeneratedExam, error) {
	exams, err := p.dbClient.GetGeneratedExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get generated exams")
		return nil, err
	}

	planEntries, err := p.dbClient.PlannedAncodes(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get planned ancodes")
		return nil, err
	}

	examsWithotSlots := make([]*model.GeneratedExam, 0)

OUTER:
	for _, exam := range exams {
		for _, planEntry := range planEntries {
			if exam.Ancode == planEntry.Ancode {
				continue OUTER
			}
		}
		examsWithotSlots = append(examsWithotSlots, exam)
	}

	// sort by student regs
	examsMap := make(map[int][]*model.GeneratedExam)
	for _, exam := range examsWithotSlots {
		exams, ok := examsMap[exam.StudentRegsCount]
		if !ok {
			exams = make([]*model.GeneratedExam, 0, 1)
		}
		examsMap[exam.StudentRegsCount] = append(exams, exam)
	}

	keys := make([]int, 0, len(examsMap))
	for key := range examsMap {
		keys = append(keys, key)
	}

	sort.Sort(sort.Reverse(sort.IntSlice(keys)))

	examsWithotSlotsSorted := make([]*model.GeneratedExam, 0, len(examsWithotSlots))
	for _, key := range keys {
		examsWithotSlotsSorted = append(examsWithotSlotsSorted, examsMap[key]...)
	}

	return examsWithotSlotsSorted, nil
}

func (p *Plexams) AncodesInPlan(ctx context.Context) ([]int, error) {
	return p.dbClient.AncodesInPlan(ctx)
}
