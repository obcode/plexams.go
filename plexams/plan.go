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

// SetExternalExamTime sets the external date/time of an exam (e.g. a MUC.DAI exam
// planned by another faculty): it computes the matching slot and stores it as the
// plan entry's externalTime. date is dd.mm.yyyy, t is HH:MM (Europe/Berlin).
func (p *Plexams) SetExternalExamTime(ctx context.Context, ancode int, date, t string) (bool, error) {
	slottime, err := time.ParseInLocation("02.01.2006 15:04", date+" "+t, time.Local)
	if err != nil {
		return false, fmt.Errorf("cannot parse date/time %q %q (expected dd.mm.yyyy and HH:MM): %w", date, t, err)
	}
	return p.AddExamToSlottime(ctx, ancode, slottime)
}

// SetExamTime places one of our own exams at an absolute start time (the source of
// truth). Any time is accepted; the derived day/slot follow from the current slot grid
// (0 when the time is outside the exam period). The GUI warns for non-standard times.
func (p *Plexams) SetExamTime(ctx context.Context, ancode int, starttime time.Time) (bool, error) {
	exam, err := p.AssembledExam(ctx, ancode)
	if err != nil || exam == nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("exam does not exist or does not need to be planned")
		return false, fmt.Errorf("exam %d does not exist or does not need to be planned", ancode)
	}
	return p.dbClient.AddExamToSlot(ctx, &model.PlanEntry{
		Starttime: &starttime,
		Ancode:    ancode,
		Locked:    false,
	})
}

func (p *Plexams) AddExamToSlottime(ctx context.Context, ancode int, slottime time.Time) (bool, error) {
	exam, err := p.GetZpaExamByAncode(ctx, ancode)
	if err != nil {
		exam, err = p.dbClient.ExternalExam(ctx, ancode)
		if err != nil {
			return false, err
		}
	} else {
		// ZPA exam
		constraints, err := p.ConstraintForAncode(ctx, ancode)
		if err != nil {
			log.Error().Err(err).Int("ancode", ancode).
				Msg("error while trying to get constraints")
			return false, err
		}
		if !constraints.NotPlannedByMe {
			err := fmt.Errorf("add exam to slot time is only allowed for exams not planned by me")
			return false, err
		}
	}
	if exam == nil {
		err = fmt.Errorf("exam with ancode %d not found", ancode)
		return false, err
	}
	log.Debug().Str("module", exam.Module).Str("main-examer", exam.MainExamer).
		Time("time", slottime).Msg("adding exam to external time")

	// External exam: the absolute time is the source of truth. Its day/slot are
	// derived on read; a time outside our exam period simply has no slot.
	return p.dbClient.AddExamToSlot(ctx, &model.PlanEntry{
		Starttime: &slottime,
		Ancode:    ancode,
		Locked:    false,
		External:  true,
	})
}

func (p *Plexams) AllowedSlots(ctx context.Context, ancode int) ([]*model.Slot, error) {
	exam, err := p.AssembledExam(ctx, ancode)
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("exam does not exist")
	}

	allSlots := p.semesterConfig.Slots

	if exam.Constraints != nil && exam.Constraints.FixedTime != nil {
		return []*model.Slot{matchSlotForFixedTime(allSlots, exam.Constraints.FixedTime)}, nil
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
	exam, err := p.AssembledExam(ctx, ancode)
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("exam does not exist")
	}

	// Group the configured slots by calendar day and order each day by start time. The
	// time-neighbours (previous / next start time on the same day) replace the former
	// slot-ordinal ±1 adjacency — identical on the fixed grid, but derived purely from time.
	slotsByDay := make(map[string][]*model.Slot)
	for _, slot := range p.semesterConfig.Slots {
		key := slot.Starttime.Format("2006-01-02")
		slotsByDay[key] = append(slotsByDay[key], slot)
	}
	for key := range slotsByDay {
		day := slotsByDay[key]
		sort.Slice(day, func(i, j int) bool { return day[i].Starttime.Before(day[j].Starttime) })
	}

	slotsWithConflicts, err := p.slotsWithConflicts(ctx, exam)
	if err != nil {
		log.Error().Err(err).Int("ancode", exam.Ancode).Msg("cannot get slots with conflicts")
		return nil, err
	}

	awkwardSlots := make([]*model.Slot, 0)
	for _, slot := range slotsWithConflicts.ToSlice() {
		day := slotsByDay[slot.Starttime.Format("2006-01-02")]
		idx := -1
		for i, s := range day {
			if s.Starttime.Equal(slot.Starttime) {
				idx = i
				break
			}
		}
		if idx < 0 {
			continue
		}
		if idx-1 >= 0 {
			awkwardSlots = append(awkwardSlots, day[idx-1])
		}
		if idx+1 < len(day) {
			awkwardSlots = append(awkwardSlots, day[idx+1])
		}
	}

	return awkwardSlots, nil
}

func (p *Plexams) slotsWithConflicts(ctx context.Context, exam *model.AssembledExam) (set.Set[model.Slot], error) {
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

// matchSlotForFixedTime returns the grid slot whose start time matches the given
// fixed time (by day, month, hour and minute), or nil. It compares start times only;
// there is no day/slot ordinal involved.
func matchSlotForFixedTime(slots []*model.Slot, time *time.Time) *model.Slot {
	for _, slot := range slots {
		if time.Day() == slot.Starttime.Day() && time.Month() == slot.Starttime.Month() &&
			time.Hour() == slot.Starttime.Hour() && time.Minute() == slot.Starttime.Minute() {
			return slot
		}
	}
	return nil
}

func getSlotsForDay(allSlots []*model.Slot, day *time.Time) []*model.Slot {
	slots := make([]*model.Slot, 0)

	for _, slot := range allSlots {
		if day.Day() == slot.Starttime.Day() && day.Month() == slot.Starttime.Month() {
			slots = append(slots, slot)
		}
	}
	return slots
}

func removeSlotsForDay(allSlots []*model.Slot, day *time.Time) []*model.Slot {
	slots := make([]*model.Slot, 0)

	for _, slot := range allSlots {
		if day.Day() != slot.Starttime.Day() || day.Month() != slot.Starttime.Month() {
			slots = append(slots, slot)
		}
	}
	return slots
}

func (p *Plexams) ExamsWithoutSlot(ctx context.Context) ([]*model.PlannedExam, error) {
	exams, err := p.dbClient.GetAssembledExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get assembled exams")
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

// ExamsAt returns all planned exams whose PlanEntry.Starttime equals starttime.
func (p *Plexams) ExamsAt(ctx context.Context, starttime time.Time) ([]*model.PlannedExam, error) {
	return p.dbClient.ExamsAt(ctx, starttime)
}

func (p *Plexams) PreExamsAt(ctx context.Context, starttime time.Time) ([]*model.PreExam, error) {
	planEntries, err := p.dbClient.PlanEntriesAt(ctx, starttime)
	if err != nil {
		log.Error().Err(err).Time("starttime", starttime).Msg("cannot get plan entries in slot")
		return nil, err
	}
	if len(planEntries) == 0 {
		return nil, nil
	}

	preExams := make([]*model.PreExam, 0, len(planEntries))
	for _, planEntry := range planEntries {
		exam, err := p.GetZPAExam(ctx, planEntry.Ancode)
		if err != nil {
			// not a ZPA exam (e.g. a MUC.DAI / non-ZPA exam) — fall back
			exam, err = p.dbClient.ExternalExam(ctx, planEntry.Ancode)
			if err != nil {
				log.Error().Err(err).Int("ancode", planEntry.Ancode).Msg("cannot get exam (neither ZPA nor non-ZPA)")
				return nil, err
			}
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
	if planEntry == nil || planEntry.Starttime == nil {
		return nil, nil
	}
	// A start time not on the grid (e.g. an external exam whose time lies outside our
	// exam period) is not an error — the exam simply has no slot.
	for _, slot := range p.semesterConfig.Slots {
		if slot.Starttime.Equal(*planEntry.Starttime) {
			return slot, nil
		}
	}

	return nil, nil
}

func (p *Plexams) LockPlan(ctx context.Context) error {
	return p.dbClient.LockPlan(ctx)
}

func (p *Plexams) LockExam(ctx context.Context, ancode int) (*model.PlanEntry, *model.AssembledExam, error) {
	planEntry, err := p.dbClient.LockExam(ctx, ancode)
	if err != nil {
		return nil, nil, err
	}
	exam, err := p.dbClient.GetAssembledExam(ctx, ancode)
	if err != nil {
		return planEntry, nil, err
	}
	return planEntry, exam, nil
}

func (p *Plexams) UnlockExam(ctx context.Context, ancode int) (*model.PlanEntry, *model.AssembledExam, error) {
	planEntry, err := p.dbClient.UnlockExam(ctx, ancode)
	if err != nil {
		return nil, nil, err
	}
	exam, err := p.dbClient.GetAssembledExam(ctx, ancode)
	if err != nil {
		return planEntry, nil, err
	}
	return planEntry, exam, nil
}
