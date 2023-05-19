package plexams

import (
	"context"
	"fmt"
	"sort"
	"time"

	set "github.com/deckarep/golang-set/v2"
	"github.com/gookit/color"
	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func (p *Plexams) AddExamToSlot(ctx context.Context, ancode int, dayNumber int, timeNumber int) (bool, error) {
	examGroup, err := p.GetExamGroupForAncode(ctx, ancode)
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot get exam group for ancode")
	}

	return p.AddExamGroupToSlot(ctx, dayNumber, timeNumber, examGroup.ExamGroupCode)
}

func (p *Plexams) GetExamGroupForAncode(ctx context.Context, ancode int) (*model.ExamGroup, error) {
	return p.dbClient.GetExamGroupForAncode(ctx, ancode)
}

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

func (p *Plexams) RmExamGroupFromSlot(ctx context.Context, examGroupCode int) (bool, error) {
	return p.dbClient.RmExamGroupFromSlot(ctx, examGroupCode)
}

func (p *Plexams) ExamGroupsInSlot(ctx context.Context, day int, time int) ([]*model.ExamGroup, error) {
	return p.dbClient.ExamGroupsInSlot(ctx, day, time)
}

func (p *Plexams) AllowedSlots(ctx context.Context, examGroupCode int) ([]*model.Slot, error) {
	if p.dbClient.ExamGroupIsLocked(ctx, examGroupCode) {
		return []*model.Slot{}, nil
	}
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

		allowedSlots = append(allowedSlots, &model.Slot{
			DayNumber:  slot.DayNumber,
			SlotNumber: slot.SlotNumber,
			Starttime:  p.getSlotTime(slot.DayNumber, slot.SlotNumber),
		})
	}

	return allowedSlots, nil
}

func (p *Plexams) AwkwardSlots(ctx context.Context, examGroupCode int) ([]*model.Slot, error) {
	if p.dbClient.ExamGroupIsLocked(ctx, examGroupCode) {
		return []*model.Slot{}, nil
	}
	examGroup, err := p.ExamGroup(ctx, examGroupCode)
	if err != nil {
		log.Error().Err(err).Int("examGroupCode", examGroupCode).Msg("exam group does not exist")
	}

	awkwardSlots := make([]*model.Slot, 0)
	for _, conflict := range examGroup.ExamGroupInfo.Conflicts {
		planEntry, err := p.dbClient.PlanEntryForExamGroup(ctx, conflict.ExamGroupCode)
		if err != nil {
			log.Error().Err(err).Int("examGroupCode", conflict.ExamGroupCode).Msg("error while trying to get plan entry")
			continue
		}

		if planEntry != nil {
			awkwardSlots = append(awkwardSlots,
				&model.Slot{
					DayNumber:  planEntry.DayNumber,
					SlotNumber: planEntry.SlotNumber - 1,
					Starttime:  time.Time{},
				},
				&model.Slot{
					DayNumber:  planEntry.DayNumber,
					SlotNumber: planEntry.SlotNumber + 1,
					Starttime:  time.Time{},
				},
			)
		}
	}

	return awkwardSlots, nil
}

func (p *Plexams) ExamGroupsWithoutSlot(ctx context.Context) ([]*model.ExamGroup, error) {
	examGroupsWithoutSlots := make([]*model.ExamGroup, 0)

	examGroups, err := p.ExamGroups(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get exam groups")
	}

	planEntries, err := p.dbClient.PlanEntries(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get plan entries")
	}

OUTER:
	for _, examGroup := range examGroups {
		for _, planEntry := range planEntries {
			if examGroup.ExamGroupCode == planEntry.ExamGroupCode {
				continue OUTER
			}
		}
		examGroupsWithoutSlots = append(examGroupsWithoutSlots, examGroup)
	}

	return examGroupsWithoutSlots, nil
}

func (p *Plexams) AllProgramsInPlan(ctx context.Context) ([]string, error) {
	examGroups, err := p.ExamGroups(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get exam groups")
	}

	programSet := set.NewSet[string]()

	for _, group := range examGroups {
		for _, program := range group.ExamGroupInfo.Programs {
			programSet.Add(program)
		}
	}

	allPrograms := programSet.ToSlice()
	sort.Strings(allPrograms)

	return allPrograms, nil
}

func (p *Plexams) AncodesInPlan(ctx context.Context) ([]int, error) {
	return p.dbClient.AncodesInPlan(ctx)
}

func (p *Plexams) ExamerInPlan(ctx context.Context) ([]*model.ExamerInPlan, error) {
	return p.dbClient.ExamerInPlan(ctx)
}

func (p *Plexams) SlotForAncode(ctx context.Context, ancode int) (*model.Slot, error) {
	examGroup, err := p.GetExamGroupForAncode(ctx, ancode)
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot get exam group for ancode")
	}

	if examGroup == nil {
		return nil, nil
	}

	return p.SlotForExamGroup(ctx, examGroup.ExamGroupCode)
}

func (p *Plexams) SlotForExamGroup(ctx context.Context, examGroupCode int) (*model.Slot, error) {
	planEntry, err := p.dbClient.PlanEntryForExamGroup(ctx, examGroupCode)
	if err != nil {
		log.Error().Err(err).Int("exam group code", examGroupCode).Msg("cannot get plan entry for exam group")
	}
	if planEntry == nil {
		return nil, nil
	}

	for _, slot := range p.semesterConfig.Slots {
		if planEntry.DayNumber == slot.DayNumber && planEntry.SlotNumber == slot.SlotNumber {
			return slot, nil
		}
	}

	return nil, fmt.Errorf("slot for exam group #%d not found", examGroupCode)
}

func (p *Plexams) PlannedExamsInSlot(ctx context.Context, day int, time int) ([]*model.PlannedExamWithNta, error) {
	examGroups, err := p.ExamGroupsInSlot(ctx, day, time)
	if err != nil {
		log.Error().Err(err).Int("day number", day).Int("slot number", time).Msg("cannot get exam group for slot")
	}
	if examGroups == nil {
		return nil, nil
	}

	ntasWithRegsByTeacher, err := p.NtasWithRegsByTeacher(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get ntas with regs")
		return nil, err
	}

	ancodeNTAMap := make(map[int][]*model.NTAWithRegs)
	for _, ntaWithRegByTeacher := range ntasWithRegsByTeacher {
		for _, ntaWithRegsByExam := range ntaWithRegByTeacher.Exams {
			ancodeNTAMap[ntaWithRegsByExam.Exam.AnCode] = ntaWithRegsByExam.Ntas
		}
	}

	plannedExams := make([]*model.PlannedExamWithNta, 0)
	for _, examGroup := range examGroups {
		for _, exam := range examGroup.Exams {
			ntas, ok := ancodeNTAMap[exam.Exam.Ancode]
			if !ok {
				ntas = nil
			}
			plannedExams = append(plannedExams, &model.PlannedExamWithNta{
				Exam:        exam.Exam,
				Constraints: exam.Constraints,
				Nta:         ntas,
			})
		}
	}
	return plannedExams, nil
}

func (p *Plexams) LockExamGroup(ctx context.Context, examGroupCode int) (*model.PlanEntry, *model.ExamGroup, error) {
	planEntry, err := p.dbClient.LockExamGroup(ctx, examGroupCode)
	if err != nil {
		return nil, nil, err
	}
	examGroup, err := p.dbClient.ExamGroup(ctx, examGroupCode)
	if err != nil {
		return planEntry, nil, err
	}
	return planEntry, examGroup, nil
}

func (p *Plexams) UnlockExamGroup(ctx context.Context, examGroupCode int) (*model.PlanEntry, *model.ExamGroup, error) {
	planEntry, err := p.dbClient.UnlockExamGroup(ctx, examGroupCode)
	if err != nil {
		return nil, nil, err
	}
	examGroup, err := p.dbClient.ExamGroup(ctx, examGroupCode)
	if err != nil {
		return planEntry, nil, err
	}
	return planEntry, examGroup, nil
}

func (p *Plexams) RemoveUnlockedExamGroupsFromPlan(ctx context.Context) (int, error) {
	return p.dbClient.RemoveUnlockedExamGroupsFromPlan(ctx)
}

func (p *Plexams) LockPlan(ctx context.Context) error {
	return p.dbClient.LockPlan(ctx)
}

func (p *Plexams) PreparePlannedExams() error {
	ctx := context.Background()
	examGroups, err := p.ExamGroups(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get exam groups")
		return err
	}

	ntas, err := p.NtasWithRegsByTeacher(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get ntas")
		return err
	}

	ntasMap := make(map[int][]*model.NTAWithRegs)
	for _, ntaWithRegByTeacher := range ntas {
		for _, ntantaWithRegByExam := range ntaWithRegByTeacher.Exams {
			ntasMap[ntantaWithRegByExam.Exam.AnCode] = ntantaWithRegByExam.Ntas
		}
	}

	doNotPublish := viper.GetIntSlice("donotpublish")
	for _, ancodeNotToPublish := range doNotPublish {
		color.Yellow.Printf("do not publish: %d\n", ancodeNotToPublish)
	}

	exams := make([]*model.ExamInPlan, 0)
OUTER:
	for _, examGroup := range examGroups {
		for _, exam := range examGroup.Exams {
			// do not include exams not planned by me
			if exam.Constraints != nil && exam.Constraints.NotPlannedByMe {
				continue
			}
			// import from other departments will sometimes be only published there
			for _, ancodeNotToPublish := range doNotPublish {
				if exam.Exam.Ancode == ancodeNotToPublish {
					continue OUTER
				}
			}
			//
			slot, err := p.SlotForAncode(ctx, exam.Exam.Ancode)
			if err != nil {
				log.Error().Err(err).Int("ancode", exam.Exam.Ancode).Msg("cannot get slot for ancode")
			}
			slot.Starttime = p.getSlotTime(slot.DayNumber, slot.SlotNumber)

			exams = append(exams, &model.ExamInPlan{
				Exam:        exam.Exam,
				Constraints: exam.Constraints,
				Nta:         ntasMap[exam.Exam.Ancode],
				Slot:        slot,
			})
		}
	}

	examsInterface := make([]interface{}, 0, len(exams))
	for _, exam := range exams {
		examsInterface = append(examsInterface, exam)
	}

	err = p.dbClient.DropAndSave(context.WithValue(ctx, db.CollectionName("collectionName"), "exams_in_plan"), examsInterface)
	if err != nil {
		log.Error().Err(err).Msg("cannot save exams in plan")
	}

	color.Green.Printf("inserted %d exams\n", len(exams))

	return nil
}

func (p *Plexams) ExamsInPlan(ctx context.Context) ([]*model.ExamInPlan, error) {
	return p.dbClient.ExamsInPlan(ctx)
}

func (p *Plexams) ExamsInSlotWithRooms(ctx context.Context, day int, time int) ([]*model.ExamWithRegsAndRooms, error) {
	examsInSlot, err := p.ExamsInSlot(ctx, day, time)
	if err != nil {
		log.Error().Err(err).Int("day", day).Int("time", time).
			Msg("cannot get exams in slot")
		return nil, err
	}

	examsInSlotWithRooms := make([]*model.ExamWithRegsAndRooms, 0, len(examsInSlot))
	for _, exam := range examsInSlot {
		rooms, err := p.dbClient.RoomsForAncode(ctx, exam.Exam.Ancode)
		if err != nil {
			log.Error().Err(err).Int("day", day).Int("time", time).Int("ancode", exam.Exam.Ancode).
				Msg("cannot get rooms for ancode")
			return nil, err
		}

		examsInSlotWithRooms = append(examsInSlotWithRooms, &model.ExamWithRegsAndRooms{
			Exam:       exam,
			NormalRegs: []*model.StudentReg{},
			NtaRegs:    []*model.NTAWithRegs{},
			Rooms:      rooms,
		})

	}

	return examsInSlotWithRooms, nil
}
