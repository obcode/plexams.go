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

func (p *Plexams) AddExamToSlot(ctx context.Context, ancode int, dayNumber int, timeNumber int, force bool) (bool, error) {
	var slot *model.Slot

	for _, s := range p.semesterConfig.Slots {
		if s.DayNumber == dayNumber && s.SlotNumber == timeNumber {
			slot = s
			break
		}
	}

	if slot == nil {
		err := fmt.Errorf("slot (%d,%d) does not exist", dayNumber, timeNumber)
		log.Error().Err(err).Int("day", dayNumber).Int("slot", timeNumber).Msg("slot does not exist")
		return false, err
	}

	// check if exam with ancode exists
	exam, err := p.GeneratedExam(ctx, ancode)
	if err != nil || exam == nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("exam does not exist or does not need to be planned")
		return false, err
	}

	if !force {
		allowedSlots, err := p.AllowedSlots(ctx, ancode)
		if err != nil {
			log.Error().Err(err).Int("ancode", ancode).Msg("cannot get allowed slots")
		}
		slotIsAllowed := false

		for _, slot := range allowedSlots {
			if slot.DayNumber == dayNumber && slot.SlotNumber == timeNumber {
				slotIsAllowed = true
				break
			}
		}
		if !slotIsAllowed {
			log.Debug().Int("day", dayNumber).Int("time", timeNumber).Int("ancode", ancode).
				Msg("slot is not allowed")
			return false, fmt.Errorf("slot (%d,%d) is not allowed for exam %d",
				dayNumber, timeNumber, ancode)
		}
	}

	return p.dbClient.AddExamToSlot(ctx, &model.PlanEntry{
		DayNumber:  slot.DayNumber,
		SlotNumber: slot.SlotNumber,
		Ancode:     ancode,
		Locked:     false,
	})
}

func (p *Plexams) PreAddExamToSlot(ctx context.Context, ancode int, dayNumber int, timeNumber int) (bool, error) {
	var slot *model.Slot

	for _, s := range p.semesterConfig.Slots {
		if s.DayNumber == dayNumber && s.SlotNumber == timeNumber {
			slot = s
			break
		}
	}

	if slot == nil {
		err := fmt.Errorf("slot (%d,%d) does not exist", dayNumber, timeNumber)
		log.Error().Err(err).Int("day", dayNumber).Int("slot", timeNumber).Msg("slot does not exist")
		return false, err
	}

	// check if exam with ancode exists
	exam, err := p.GetZPAExam(ctx, ancode)
	if err != nil || exam == nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("zpa exam does not exist")
		return false, err
	}

	return p.dbClient.AddExamToSlot(ctx, &model.PlanEntry{
		DayNumber:  slot.DayNumber,
		SlotNumber: slot.SlotNumber,
		Ancode:     ancode,
		Locked:     false,
	})
}

func (p *Plexams) AllowedSlots(ctx context.Context, ancode int) ([]*model.Slot, error) {
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

	for _, slot := range p.semesterConfig.ForbiddenSlots {
		slotsWithConflicts.Add(*slot)
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
		if day.Local().Day() == slot.Starttime.Local().Day() && day.Local().Month() == slot.Starttime.Local().Month() {
			slots = append(slots, slot)
		}
	}
	return slots
}

func removeSlotsForDay(allSlots []*model.Slot, day *time.Time) []*model.Slot {
	slots := make([]*model.Slot, 0)

	for _, slot := range allSlots {
		if day.Local().Day() != slot.Starttime.Local().Day() || day.Local().Month() != slot.Starttime.Local().Month() {
			slots = append(slots, slot)
		}
	}
	return slots
}

func (p *Plexams) ExamsWithoutSlot(ctx context.Context) ([]*model.PlannedExam, error) {
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

	examsWithotSlots := make([]*model.PlannedExam, 0)

OUTER:
	for _, exam := range exams {
		for _, planEntry := range planEntries {
			if exam.Ancode == planEntry.Ancode {
				continue OUTER
			}
		}
		examsWithotSlots = append(examsWithotSlots, &model.PlannedExam{
			Ancode:           exam.Ancode,
			ZpaExam:          exam.ZpaExam,
			PrimussExams:     exam.PrimussExams,
			Constraints:      exam.Constraints,
			Conflicts:        exam.Conflicts,
			StudentRegsCount: exam.StudentRegsCount,
			Ntas:             exam.Ntas,
			MaxDuration:      exam.MaxDuration,
		})
	}

	// sort by student regs
	examsMap := make(map[int][]*model.PlannedExam)
	for _, exam := range examsWithotSlots {
		exams, ok := examsMap[exam.StudentRegsCount]
		if !ok {
			exams = make([]*model.PlannedExam, 0, 1)
		}
		examsMap[exam.StudentRegsCount] = append(exams, exam)
	}

	keys := make([]int, 0, len(examsMap))
	for key := range examsMap {
		keys = append(keys, key)
	}

	sort.Sort(sort.Reverse(sort.IntSlice(keys)))

	examsWithotSlotsSorted := make([]*model.PlannedExam, 0, len(examsWithotSlots))
	for _, key := range keys {
		examsWithotSlotsSorted = append(examsWithotSlotsSorted, examsMap[key]...)
	}

	return examsWithotSlotsSorted, nil
}

func (p *Plexams) AncodesInPlan(ctx context.Context) ([]int, error) {
	return p.dbClient.AncodesInPlan(ctx)
}

// Deprecated: rm me
func (p *Plexams) ExamsInSlot(ctx context.Context, day int, time int) ([]*model.ExamInPlan, error) {
	return p.dbClient.ExamsInSlot(ctx, day, time)
}

func (p *Plexams) GetExamsInSlot(ctx context.Context, day int, time int) ([]*model.PlannedExam, error) {
	return p.dbClient.GetExamsInSlot(ctx, day, time)
}

func (p *Plexams) PreExamsInSlot(ctx context.Context, day int, time int) ([]*model.PreExam, error) {
	planEntries, err := p.dbClient.GetPlanEntriesInSlot(ctx, day, time)
	if err != nil {
		log.Error().Err(err).Int("day", day).Int("time", time).Msg("cannot get plan entries in slot")
		return nil, err
	}
	if len(planEntries) == 0 {
		return nil, nil
	}

	preExams := make([]*model.PreExam, 0, len(planEntries))
	for _, planEntry := range planEntries {
		exam, err := p.GetZPAExam(ctx, planEntry.Ancode)
		if err != nil {
			log.Error().Err(err).Int("ancode", planEntry.Ancode).Msg("cannot get exam")
			return nil, err
		}
		constraints, err := p.ConstraintForAncode(ctx, planEntry.Ancode)
		if err != nil {
			log.Error().Err(err).Int("ancode", planEntry.Ancode).Msg("cannot get constraints")
			return nil, err
		}
		planEntry, err := p.dbClient.PlanEntry(ctx, exam.AnCode)
		if err != nil {
			log.Error().Err(err).Int("ancode", exam.AnCode).Msg("cannot get plan entry")
		}
		preExams = append(preExams, &model.PreExam{
			ZpaExam:     exam,
			Constraints: constraints,
			PlanEntry:   planEntry,
		})
	}

	return preExams, nil
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
