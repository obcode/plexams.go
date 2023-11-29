package plexams

import (
	"context"
	"fmt"
	"sort"
	"time"

	set "github.com/deckarep/golang-set/v2"
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
	if p.dbClient.ExamIsLocked(ctx, ancode) {
		slot, err := p.SlotForAncode(ctx, ancode)
		if err != nil {
			log.Error().Err(err).Int("ancode", ancode).Msg("exam is locked, but got an error on getting slot")
			return nil, err
		}
		return []*model.Slot{slot}, nil
	}

	exam, err := p.GeneratedExam(ctx, ancode)
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("exam does not exist")
	}

	allSlots := p.semesterConfig.Slots

	if exam.Constraints != nil && exam.Constraints.FixedTime != nil {
		return []*model.Slot{getSlotForTime(allSlots, exam.Constraints.FixedTime)}, nil
	}

	if exam.Constraints != nil && exam.Constraints.FixedDay != nil {
		return getSlotsForDay(allSlots, exam.Constraints.FixedDay), nil
	}

	allowedSlots := make([]*model.Slot, 0)

	if exam.Constraints != nil && exam.Constraints.PossibleDays != nil {
		for _, day := range exam.Constraints.PossibleDays {
			allowedSlots = append(allowedSlots, getSlotsForDay(allSlots, day)...)
		}
	} else {
		allowedSlots = allSlots
	}

	if exam.Constraints != nil && exam.Constraints.ExcludeDays != nil {
		for _, day := range exam.Constraints.ExcludeDays {
			allowedSlots = removeSlotsForDay(allowedSlots, day)
		}
	}

	// TODO: recalculate from conflicts

	allowedSlotsWithConflicts := make([]*model.Slot, 0, len(allowedSlots))

	slotsWithConflicts, err := p.slotsWithConflicts(ctx, exam)
	if err != nil {
		log.Error().Err(err).Int("ancode", exam.Ancode).Msg("cannot get slots with conflicts")
		return nil, err
	}

	for _, slot := range allowedSlots {
		if !slotsWithConflicts.Contains(*slot) {
			allowedSlotsWithConflicts = append(allowedSlotsWithConflicts, slot)
		}
	}

	return allowedSlotsWithConflicts, nil
}

func (p *Plexams) AwkwardSlots(ctx context.Context, ancode int) ([]*model.Slot, error) {
	exam, err := p.GeneratedExam(ctx, ancode)
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("exam does not exist")
	}

	type SlotID struct {
		DayNumber, SlotNumber int
	}

	allSlotsMap := make(map[SlotID]*model.Slot)
	for _, slot := range p.semesterConfig.Slots {
		allSlotsMap[SlotID{slot.DayNumber, slot.SlotNumber}] = slot
	}

	slotsWithConflicts, err := p.slotsWithConflicts(ctx, exam)
	if err != nil {
		log.Error().Err(err).Int("ancode", exam.Ancode).Msg("cannot get slots with conflicts")
		return nil, err
	}

	awkwardSlots := make([]*model.Slot, 0)
	for _, slot := range slotsWithConflicts.ToSlice() {
		slotMinus1, ok := allSlotsMap[SlotID{slot.DayNumber, slot.SlotNumber - 1}]
		if ok {
			awkwardSlots = append(awkwardSlots, slotMinus1)
		}
		slotPlus1, ok := allSlotsMap[SlotID{slot.DayNumber, slot.SlotNumber + 1}]
		if ok {
			awkwardSlots = append(awkwardSlots, slotPlus1)
		}
	}

	return awkwardSlots, nil
}

func (p *Plexams) slotsWithConflicts(ctx context.Context, exam *model.GeneratedExam) (set.Set[model.Slot], error) {
	slotSet := set.NewSet[model.Slot]()
	for _, conflict := range exam.Conflicts {
		slot, err := p.SlotForAncode(ctx, conflict.Ancode)
		if err != nil {
			log.Error().Err(err).Int("ancode", conflict.Ancode).Msg("cannot get slot for ancode")
			return nil, err
		}
		if slot != nil {
			slotSet.Add(*slot)
		}
	}
	return slotSet, nil
}

func getSlotForTime(slots []*model.Slot, time *time.Time) *model.Slot {
	for _, slot := range slots {
		if time.Local().Day() == slot.Starttime.Day() && time.Local().Month() == slot.Starttime.Month() &&
			time.Local().Hour() == slot.Starttime.Local().Hour() && time.Local().Minute() == slot.Starttime.Local().Minute() {
			return slot
		}
	}
	return nil
}

func getSlotsForDay(allSlots []*model.Slot, day *time.Time) []*model.Slot {
	slots := make([]*model.Slot, 0)

	for _, slot := range allSlots {
		if day.Local().Day() == slot.Starttime.Day() && day.Local().Month() == slot.Starttime.Month() {
			slots = append(slots, slot)
		}
	}
	return slots
}

func removeSlotsForDay(allSlots []*model.Slot, day *time.Time) []*model.Slot {
	slots := make([]*model.Slot, 0)

	for _, slot := range allSlots {
		if !(day.Day() == slot.Starttime.Day() && day.Month() == slot.Starttime.Month()) {
			slots = append(slots, slot)
		}
	}
	return slots
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

func (p *Plexams) ExamsInSlot(ctx context.Context, day int, time int) ([]*model.ExamInPlan, error) {
	return p.dbClient.ExamsInSlot(ctx, day, time)
}

func (p *Plexams) GetExamsInSlot(ctx context.Context, day int, time int) ([]*model.GeneratedExam, error) {
	return p.dbClient.GetExamsInSlot(ctx, day, time)
}

func (p *Plexams) SlotForAncode(ctx context.Context, ancode int) (*model.Slot, error) {
	planEntry, err := p.dbClient.PlanEntry(ctx, ancode)
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot get plan entry for exam")
		return nil, err
	}
	if planEntry == nil {
		return nil, nil
	}

	for _, slot := range p.semesterConfig.Slots {
		if planEntry.DayNumber == slot.DayNumber && planEntry.SlotNumber == slot.SlotNumber {
			return slot, nil
		}
	}

	return nil, fmt.Errorf("slot for exam #%d not found", ancode)
}

func (p *Plexams) LockPlan(ctx context.Context) error {
	return p.dbClient.LockPlan(ctx)
}
