package plexams

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	set "github.com/deckarep/golang-set/v2"
	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func (p *Plexams) InvigilatorsWithReq(ctx context.Context) ([]*model.Invigilator, error) {
	teachers, err := p.getInvigilators(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get teachers")
		return nil, err
	}

	invigilators := make([]*model.Invigilator, 0, len(teachers))
	for _, teacher := range teachers {
		invigilatorConstraints := viper.Get(fmt.Sprintf("invigilatorConstraints.%d", teacher.ID))
		if invigilatorConstraints != nil {
			if viper.GetBool(fmt.Sprintf("invigilatorConstraints.%d.isNotInvigilator", teacher.ID)) {
				log.Debug().Str("name", teacher.Shortname).Msg("is not invigilator")
				continue
			}
		}

		reqs, err := p.dbClient.GetInvigilatorRequirements(ctx, teacher.ID)
		if err != nil {
			log.Error().Err(err).Str("teacher", teacher.Shortname).Msg("cannot get requirements for teacher")
		}

		var invigReqs *model.InvigilatorRequirements
		if reqs != nil {
			invigilatorConstraints := viper.Get(fmt.Sprintf("invigilatorConstraints.%d", teacher.ID))
			var onlyInSlots []*model.Slot
			if invigilatorConstraints != nil {
				excludedDates := viper.GetStringSlice(fmt.Sprintf("invigilatorConstraints.%d.excludedDates", teacher.ID))
				if len(excludedDates) > 0 {
					log.Debug().Interface("excludedDates", excludedDates).Str("name", teacher.Shortname).Msg("found in config")
					reqs.ExcludedDates = append(reqs.ExcludedDates, excludedDates...)
				}

				onlyInSlotsCfg := viper.Get(fmt.Sprintf("invigilatorConstraints.%d.onlyInSlots", teacher.ID))
				if onlyInSlotsCfg != nil {
					slotsSlice := onlyInSlotsCfg.([]interface{})
					onlyInSlots = make([]*model.Slot, 0, len(slotsSlice))
					for _, slot := range slotsSlice {
						slotSlice := slot.([]interface{})
						onlyInSlots = append(onlyInSlots, &model.Slot{
							DayNumber:  slotSlice[0].(int),
							SlotNumber: slotSlice[1].(int),
							Starttime:  time.Time{},
						})
					}
					log.Debug().Interface("slots", onlyInSlots).Str("name", teacher.Shortname).Msg("onlyInSlots found")
				}
			}

			excludedDates := make([]*time.Time, 0, len(reqs.ExcludedDates))
			for _, day := range reqs.ExcludedDates {
				loc, _ := time.LoadLocation("Europe/Berlin")
				t, err := time.ParseInLocation("02.01.06", day, loc)
				if err != nil {
					log.Error().Err(err).Str("day", day).Msg("cannot parse date")
				} else {
					excludedDates = append(excludedDates, &t)
				}
			}

			examDateTimes := make([]*time.Time, 0)
			exams, err := p.GeneratedExamsForExamer(ctx, teacher.ID)
			if err != nil {
				log.Error().Err(err).Str("name", teacher.Shortname).Msg("cannit get exams by main examer")
			} else {
				for _, exam := range exams {
					planEntry, err := p.dbClient.PlanEntry(ctx, exam.Ancode)
					if err != nil {
						log.Error().Err(err).Int("ancode", exam.Ancode).Msg("cannot get plan entry for ancode")
					}
					if planEntry != nil {
						starttime := p.getSlotTime(planEntry.DayNumber, planEntry.SlotNumber)
						examDateTimes = append(examDateTimes, &starttime)
					}
				}
			}

			factor := 1.0 * reqs.PartTime

			if reqs.OvertimeThisSemester != 0 {
				factor *= reqs.OvertimeThisSemester
			}

			if reqs.FreeSemester == 0.5 {
				factor *= 0.5
			}

			if reqs.FreeSemester == 1.0 ||
				reqs.OvertimeLastSemester != 0 && reqs.FreeSemester == 0.5 {
				factor = 0.0
			}

			log.Debug().Str("name", teacher.Shortname).Float64("faktor", factor).
				Msg("Faktor für Aufsichten")

			invigReqs = &model.InvigilatorRequirements{
				ExcludedDates:          excludedDates,
				ExcludedDays:           p.datesToDay(excludedDates),
				ExamDateTimes:          examDateTimes,
				ExamDays:               p.datesToDay(examDateTimes),
				PartTime:               reqs.PartTime,
				OralExamsContribution:  reqs.OralExamsContribution,
				LiveCodingContribution: reqs.LivecodingContribution,
				MasterContribution:     reqs.MasterContribution,
				FreeSemester:           reqs.FreeSemester,
				OvertimeLastSemester:   reqs.OvertimeLastSemester,
				OvertimeThisSemester:   reqs.OvertimeThisSemester,
				AllContributions:       reqs.OralExamsContribution + reqs.LivecodingContribution + reqs.MasterContribution,
				Factor:                 factor,
				OnlyInSlots:            onlyInSlots,
			}
		}

		invigilators = append(invigilators, &model.Invigilator{
			Teacher:      teacher,
			Requirements: invigReqs,
		})
	}

	return invigilators, nil
}

func (p *Plexams) datesToDay(dates []*time.Time) []int {
	days := set.NewSet[int]()
	for _, date := range dates {
		for _, day := range p.semesterConfig.Days {
			if day.Date.Month() == date.Month() && day.Date.Day() == date.Day() {
				days.Add(day.Number)
			}
		}
	}
	daysSlice := days.ToSlice()
	sort.Ints(daysSlice)
	return daysSlice
}

func (p *Plexams) GetInvigilationTodos(ctx context.Context) (*model.InvigilationTodos, error) {
	todos, err := p.dbClient.GetInvigilationTodos(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get invigilation todos")
		return nil, err
	}
	if todos == nil {
		return p.PrepareInvigilationTodos(ctx)
	}

	err = p.AddInvigilatorsToInvigilationTodos(ctx, todos)
	if err != nil {
		log.Error().Err(err).Msg("cannot add invigilators to invigilation todos")
		return nil, err
	}

	err = p.dbClient.CacheInvigilatorTodos(ctx, todos)
	if err != nil {
		log.Error().Err(err).Msg("cannot cache invigilation todos")
		return nil, err
	}

	return todos, nil
}

func (p *Plexams) PrepareInvigilationTodos(ctx context.Context) (*model.InvigilationTodos, error) {
	selfInvigilations, err := p.MakeSelfInvigilations(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get self invigilations")
	}

	todos := model.InvigilationTodos{}

	for _, slot := range p.semesterConfig.Slots {
		roomsInSlot, err := p.PlannedRoomsInSlot(ctx, slot.DayNumber, slot.SlotNumber)
		if err != nil {
			log.Error().Err(err).Int("day", slot.DayNumber).Int("time", slot.SlotNumber).
				Msg("cannot get rooms for slot")
		} else {
			if len(roomsInSlot) == 0 {
				continue
			}
			roomMap := make(map[string]int)
		OUTER:
			for _, room := range roomsInSlot {
				for _, selfInvigilation := range selfInvigilations {
					if selfInvigilation.Slot.DayNumber == slot.DayNumber &&
						selfInvigilation.Slot.SlotNumber == slot.SlotNumber &&
						*selfInvigilation.RoomName == room.RoomName {
						log.Debug().Int("day", slot.DayNumber).Int("slot", slot.SlotNumber).Str("room", room.RoomName).
							Msg("found self invigilation")
						continue OUTER
					}
				}
				maxDuration, ok := roomMap[room.RoomName]
				if !ok || maxDuration < room.Duration {
					roomMap[room.RoomName] = room.Duration
				}
			}

			for _, maxDuration := range roomMap {
				todos.SumExamRooms += maxDuration
			}
			todos.SumReserve += 60 // FIXME: Maybe some other time? half of max duration in slot?
		}
	}

	reqs, err := p.InvigilatorsWithReq(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get invigilators with regs")
	}

	todos.Invigilators = reqs

	for _, invigilator := range reqs {
		if invigilator.Requirements != nil {
			todos.SumOtherContributions += invigilator.Requirements.AllContributions
		}
	}

	todos.InvigilatorCount = len(reqs)
	adjustedInvigilatorCount := 0.0

	for _, invigilator := range reqs {
		if invigilator.Requirements != nil {
			adjustedInvigilatorCount += invigilator.Requirements.Factor
		}
	}

	todos.TodoPerInvigilator = int(math.Ceil(float64(todos.SumExamRooms+todos.SumReserve+todos.SumOtherContributions) / adjustedInvigilatorCount))

	sumOtherContributionsOvertimeCutted := 0
	for _, invigilator := range reqs {
		if invigilator.Requirements != nil {
			if otherContributions := invigilator.Requirements.OralExamsContribution +
				invigilator.Requirements.LiveCodingContribution +
				invigilator.Requirements.MasterContribution; otherContributions > todos.TodoPerInvigilator {
				sumOtherContributionsOvertimeCutted += todos.TodoPerInvigilator
			} else {
				sumOtherContributionsOvertimeCutted += otherContributions
			}
		}
	}
	todos.SumOtherContributionsOvertimeCutted = sumOtherContributionsOvertimeCutted
	todos.TodoPerInvigilatorOvertimeCutted = int(math.Ceil(float64(todos.SumExamRooms+todos.SumReserve+sumOtherContributionsOvertimeCutted) / adjustedInvigilatorCount))

	for _, invigilator := range todos.Invigilators {

		enough := false
		totalMinutes := todos.TodoPerInvigilatorOvertimeCutted
		if invigilator.Requirements != nil {
			totalMinutes = int(float64(totalMinutes) * invigilator.Requirements.Factor)
		}

		if invigilator.Requirements != nil {
			totalMinutes -= invigilator.Requirements.AllContributions

			if totalMinutes < 0 {
				totalMinutes = 0
				enough = true
			}
		}

		invigilationsForInvigilator, err := p.dbClient.InvigilationsForInvigilator(ctx, invigilator.Teacher.ID)
		if err != nil {
			log.Error().Err(err).Str("invigilator", invigilator.Teacher.Shortname).
				Msg("cannot get invigilations")
		}

		invigilationSet := set.NewSet[int]()
		doingMinutes := 0

		for _, invigilation := range invigilationsForInvigilator {
			invigilationSet.Add(invigilation.Slot.DayNumber)
			if !invigilation.IsSelfInvigilation {
				doingMinutes += invigilation.Duration
			}
		}
		invigilationDays := invigilationSet.ToSlice()
		sort.Ints(invigilationDays)

		invigilator.Todos = &model.InvigilatorTodos{
			TotalMinutes:     totalMinutes,
			DoingMinutes:     doingMinutes,
			Enough:           enough,
			InvigilationDays: invigilationDays,
			Invigilations:    invigilationsForInvigilator,
		}
	}

	err = p.dbClient.CacheInvigilatorTodos(ctx, &todos)
	if err != nil {
		log.Error().Err(err).Msg("cannot cache invigilation todos")
		return &todos, err
	}

	return &todos, nil
}

func (p *Plexams) AddInvigilatorsToInvigilationTodos(ctx context.Context, todos *model.InvigilationTodos) error {
	reqs, err := p.InvigilatorsWithReq(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get invigilators with regs")
		return err
	}

	todos.Invigilators = reqs

	for _, invigilator := range todos.Invigilators {

		enough := false
		totalMinutes := todos.TodoPerInvigilatorOvertimeCutted
		if invigilator.Requirements != nil {
			totalMinutes = int(float64(totalMinutes) * invigilator.Requirements.Factor)
		}

		if invigilator.Requirements != nil {
			totalMinutes -= invigilator.Requirements.AllContributions

			if totalMinutes < 0 {
				totalMinutes = 0
				enough = true
			}
		}

		invigilationsForInvigilator, err := p.dbClient.InvigilationsForInvigilator(ctx, invigilator.Teacher.ID)
		if err != nil {
			log.Error().Err(err).Str("invigilator", invigilator.Teacher.Shortname).
				Msg("cannot get invigilations")
			return err
		}

		invigilationSet := set.NewSet[int]()
		doingMinutes := 0

		for _, invigilation := range invigilationsForInvigilator {
			invigilationSet.Add(invigilation.Slot.DayNumber)
			if !invigilation.IsSelfInvigilation {
				doingMinutes += invigilation.Duration
			}
		}
		invigilationDays := invigilationSet.ToSlice()
		sort.Ints(invigilationDays)

		invigilator.Todos = &model.InvigilatorTodos{
			TotalMinutes:     totalMinutes,
			DoingMinutes:     doingMinutes,
			Enough:           enough,
			InvigilationDays: invigilationDays,
			Invigilations:    invigilationsForInvigilator,
		}
	}

	return nil
}

// Deprecated: split to other functions
func (p *Plexams) InvigilationTodos(ctx context.Context) (*model.InvigilationTodos, error) {
	// selfInvigilations, err := p.MakeSelfInvigilations(ctx)
	// if err != nil {
	// 	log.Error().Err(err).Msg("cannot get self invigilations")
	// }

	// todos := model.InvigilationTodos{}

	// for _, slot := range p.semesterConfig.Slots {
	// 	roomsInSlot, err := p.PlannedRoomsInSlot(ctx, slot.DayNumber, slot.SlotNumber)
	// 	if err != nil {
	// 		log.Error().Err(err).Int("day", slot.DayNumber).Int("time", slot.SlotNumber).
	// 			Msg("cannot get rooms for slot")
	// 	} else {
	// 		if len(roomsInSlot) == 0 {
	// 			continue
	// 		}
	// 		roomMap := make(map[string]int)
	// 	OUTER:
	// 		for _, room := range roomsInSlot {
	// 			for _, selfInvigilation := range selfInvigilations {
	// 				if selfInvigilation.Slot.DayNumber == slot.DayNumber &&
	// 					selfInvigilation.Slot.SlotNumber == slot.SlotNumber &&
	// 					*selfInvigilation.RoomName == room.RoomName {
	// 					log.Debug().Int("day", slot.DayNumber).Int("slot", slot.SlotNumber).Str("room", room.RoomName).
	// 						Msg("found self invigilation")
	// 					continue OUTER
	// 				}
	// 			}
	// 			maxDuration, ok := roomMap[room.RoomName]
	// 			if !ok || maxDuration < room.Duration {
	// 				roomMap[room.RoomName] = room.Duration
	// 			}
	// 		}

	// 		for _, maxDuration := range roomMap {
	// 			todos.SumExamRooms += maxDuration
	// 		}
	// 		todos.SumReserve += 60 // FIXME: Maybe some other time? half of max duration in slot?
	// 	}
	// }

	// reqs, err := p.InvigilatorsWithReq(ctx)
	// if err != nil {
	// 	log.Error().Err(err).Msg("cannot get invigilators with regs")
	// }

	// todos.Invigilators = reqs

	// for _, invigilator := range reqs {
	// 	if invigilator.Requirements != nil {
	// 		todos.SumOtherContributions += invigilator.Requirements.AllContributions
	// 	}
	// }

	// todos.InvigilatorCount = len(reqs)
	// adjustedInvigilatorCount := 0.0

	// for _, invigilator := range reqs {
	// 	if invigilator.Requirements != nil {
	// 		adjustedInvigilatorCount += invigilator.Requirements.Factor
	// 	}
	// }

	// todos.TodoPerInvigilator = int(math.Ceil(float64(todos.SumExamRooms+todos.SumReserve+todos.SumOtherContributions) / adjustedInvigilatorCount))

	// sumOtherContributionsOvertimeCutted := 0
	// for _, invigilator := range reqs {
	// 	if invigilator.Requirements != nil {
	// 		if otherContributions := invigilator.Requirements.OralExamsContribution +
	// 			invigilator.Requirements.LiveCodingContribution +
	// 			invigilator.Requirements.MasterContribution; otherContributions > todos.TodoPerInvigilator {
	// 			sumOtherContributionsOvertimeCutted += todos.TodoPerInvigilator
	// 		} else {
	// 			sumOtherContributionsOvertimeCutted += otherContributions
	// 		}
	// 	}
	// }
	// todos.SumOtherContributionsOvertimeCutted = sumOtherContributionsOvertimeCutted
	// todos.TodoPerInvigilatorOvertimeCutted = int(math.Ceil(float64(todos.SumExamRooms+todos.SumReserve+sumOtherContributionsOvertimeCutted) / adjustedInvigilatorCount))

	// for _, invigilator := range todos.Invigilators {

	// 	enough := false
	// 	totalMinutes := todos.TodoPerInvigilatorOvertimeCutted
	// 	if invigilator.Requirements != nil {
	// 		totalMinutes = int(float64(totalMinutes) * invigilator.Requirements.Factor)
	// 	}

	// 	if invigilator.Requirements != nil {
	// 		totalMinutes -= invigilator.Requirements.AllContributions

	// 		if totalMinutes < 0 {
	// 			totalMinutes = 0
	// 			enough = true
	// 		}
	// 	}

	// 	invigilationsForInvigilator, err := p.dbClient.InvigilationsForInvigilator(ctx, invigilator.Teacher.ID)
	// 	if err != nil {
	// 		log.Error().Err(err).Str("invigilator", invigilator.Teacher.Shortname).
	// 			Msg("cannot get invigilations")
	// 	}

	// 	invigilationSet := set.NewSet[int]()
	// 	doingMinutes := 0

	// 	for _, invigilation := range invigilationsForInvigilator {
	// 		invigilationSet.Add(invigilation.Slot.DayNumber)
	// 		if !invigilation.IsSelfInvigilation {
	// 			doingMinutes += invigilation.Duration
	// 		}
	// 	}
	// 	invigilationDays := invigilationSet.ToSlice()
	// 	sort.Ints(invigilationDays)

	// 	invigilator.Todos = &model.InvigilatorTodos{
	// 		TotalMinutes:     totalMinutes,
	// 		DoingMinutes:     doingMinutes,
	// 		Enough:           enough,
	// 		InvigilationDays: invigilationDays,
	// 		Invigilations:    invigilationsForInvigilator,
	// 	}
	// }

	// return &todos, nil
	return nil, nil
}

func (p *Plexams) InvigilatorsForDay(ctx context.Context, day int) (*model.InvigilatorsForDay, error) {
	invigilationTodos, err := p.dbClient.GetInvigilationTodos(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get invigilation todos")
		return nil, err
	}

	want := make([]*model.Invigilator, 0)
	can := make([]*model.Invigilator, 0)

	for _, invigilator := range invigilationTodos.Invigilators {
		wantDay, canDay := dayOkForInvigilator(day, invigilator)
		if wantDay {
			want = append(want, invigilator)
		} else if canDay {
			can = append(can, invigilator)
		}
	}

	return &model.InvigilatorsForDay{
		Want: want,
		Can:  can,
	}, nil
}

func dayOkForInvigilator(day int, invigilator *model.Invigilator) (wantDay, canDay bool) {
	// day in exlude days?
	for _, excludedDay := range invigilator.Requirements.ExcludedDays {
		if day == excludedDay {
			return false, false
		}
	}
	for _, examDay := range invigilator.Requirements.ExamDays {
		if day == examDay {
			return true, true
		}
	}
	for _, invigilationDay := range invigilator.Todos.InvigilationDays {
		if day == invigilationDay {
			return true, true
		}
	}
	return false, true
}

func (p *Plexams) PrepareSelfInvigilation() error {
	ctx := context.Background()
	selfInvigilations, err := p.MakeSelfInvigilations(ctx)
	if err != nil {
		return err
	}

	toSave := make([]interface{}, 0, len(selfInvigilations))
	for _, invig := range selfInvigilations {
		log.Debug().Interface("invigilation", invig).Msg("adding invigilation to slice")
		toSave = append(toSave, invig)
	}

	log.Debug().Interface("ivigilations", toSave).Msg("saving invigilations")

	return p.dbClient.DropAndSave(context.WithValue(ctx, db.CollectionName("collectionName"), "invigilations_self"), toSave)
}

func (p *Plexams) MakeSelfInvigilations(ctx context.Context) ([]*model.Invigilation, error) {
	invigilators, err := p.InvigilatorsWithReq(ctx)
	if err != nil || invigilators == nil {
		log.Error().Err(err).Msg("cannot get invigilators")
		return nil, err
	}

	log.Debug().Interface("invigilators", invigilators).Msg("got invigilators")

	invigilatorMap := make(map[int]*model.Invigilator)
	for _, invigilator := range invigilators {
		invigilatorMap[invigilator.Teacher.ID] = invigilator
	}

	type key struct {
		roomName string
		day      int
		slot     int
	}

	invigilationsMap := make(map[key][]*model.Invigilation)

	for _, slot := range p.semesterConfig.Slots {
		examsInSlot, err := p.GetExamsInSlot(ctx, slot.DayNumber, slot.SlotNumber)
		if err != nil {
			log.Error().Err(err).Int("day", slot.DayNumber).Int("time", slot.SlotNumber).
				Msg("cannot get exams in slot")
			continue
		}
		examerWithExams := make(map[int][]*model.PlannedExam)
	OUTER:
		for _, exam := range examsInSlot {
			invigilator, ok := invigilatorMap[exam.ZpaExam.MainExamerID]

			if !ok {
				log.Debug().Str("name", exam.ZpaExam.MainExamer).Msg("ist keine Aufsicht")
				continue
			}
			if exam.Constraints != nil && exam.Constraints.NotPlannedByMe {
				continue
			}
			if len(exam.PlannedRooms) == 0 {
				continue
			}
			if invigilator.Requirements != nil {
				for _, day := range invigilator.Requirements.ExcludedDays {
					if day == exam.PlanEntry.DayNumber {
						log.Debug().Str("name", exam.ZpaExam.MainExamer).Interface("slot", exam.PlanEntry).
							Msg("Tag ist gesperrt für Aufsicht")
						continue OUTER
					}
				}
			}
			exams, ok := examerWithExams[exam.ZpaExam.MainExamerID]
			if !ok {
				examerWithExams[exam.ZpaExam.MainExamerID] = []*model.PlannedExam{exam}
			} else {
				examerWithExams[exam.ZpaExam.MainExamerID] = append(exams, exam)
			}
		}

		for examer, exams := range examerWithExams {
			roomNames := set.NewSet[string]()
			for _, exam := range exams {
				for _, room := range exam.PlannedRooms {
					roomNames.Add(room.RoomName)
				}
			}

			if roomNames.Cardinality() == 1 {
				log.Debug().Int("examerid", examer).Interface("room", roomNames).Interface("slot", slot).
					Msg("found self invigilation")
				key := key{
					roomName: roomNames.ToSlice()[0],
					day:      slot.DayNumber,
					slot:     slot.SlotNumber,
				}
				invigilationsForKey, ok := invigilationsMap[key]
				if !ok {
					invigilationsForKey = make([]*model.Invigilation, 0, 1)
				}
				invigilationsMap[key] = append(invigilationsForKey, &model.Invigilation{
					RoomName:           &roomNames.ToSlice()[0],
					Duration:           0, // FIXME: ?? self-invigilation does not count
					InvigilatorID:      examer,
					Slot:               slot,
					IsReserve:          false,
					IsSelfInvigilation: true,
				})
			}
		}

	}

	invigilations := make([]*model.Invigilation, 0)
	for _, invigs := range invigilationsMap {
		// if len(invigs) == 1 {
		// 	invigilations = append(invigilations, invigs...)
		// } else {
		if len(invigs) > 1 {
			log.Debug().Interface("invigs", invigs).Msg("found more self invigs")
		}
		// 	// TODO: find examer with most studs in room
		// 	// for _, invig := range invigs {

		// 	// }
		// }
		invigilations = append(invigilations, invigs[0])
	}

	log.Debug().Int("count", len(invigilations)).Msg("found self invigilations")

	return invigilations, nil
}

// TODO: rewrite me
func (p *Plexams) RoomsWithInvigilationsForSlot(ctx context.Context, day int, time int) (*model.InvigilationSlot, error) {
	rooms, err := p.PlannedRoomsInSlot(ctx, day, time)
	if err != nil {
		log.Error().Err(err).Int("day", day).Int("time", time).
			Msg("cannot get rooms for slot")
		return nil, err
	}

	if len(rooms) == 0 {
		return nil, nil // okay?
	}

	reserve, err := p.dbClient.GetInvigilatorInSlot(ctx, "reserve", day, time)
	if err != nil {
		log.Error().Err(err).Int("day", day).Int("time", time).
			Msg("cannot get reserve for slot")
		return nil, err
	}

	slot := &model.InvigilationSlot{
		Reserve:               reserve,
		RoomsWithInvigilators: []*model.RoomWithInvigilator{},
	}

	roomMap := make(map[string][]*model.PlannedRoom)

	for _, room := range rooms {
		roomsForExam, ok := roomMap[room.RoomName]
		if !ok {
			roomsForExam = make([]*model.PlannedRoom, 0, 1)
		}
		roomMap[room.RoomName] = append(roomsForExam, room)
	}

	keys := make([]string, 0, len(roomMap))
	for k := range roomMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, name := range keys {
		roomsForExam := roomMap[name]
		invigilator, err := p.dbClient.GetInvigilatorForRoom(ctx, name, day, time)
		if err != nil {
			log.Error().Err(err).Int("day", day).Int("slot", time).Str("room", name).
				Msg("cannot get invigilator for rooms in slot")
		}

		roomAndExams := make([]*model.RoomAndExam, 0)
		maxDuration := 0
		studentCount := 0
		for _, roomForExam := range roomsForExam {
			exam, err := p.dbClient.GetZpaExamByAncode(ctx, roomForExam.Ancode)
			if err != nil {
				log.Error().Err(err).Int("ancode", roomForExam.Ancode).
					Msg("cannot get zpa exam")
				return nil, err
			}
			roomAndExams = append(roomAndExams, &model.RoomAndExam{
				Room: roomForExam,
				Exam: exam,
			})
			if roomForExam.Duration > maxDuration {
				maxDuration = roomForExam.Duration
			}
			studentCount += len(roomForExam.StudentsInRoom)
		}

		slot.RoomsWithInvigilators = append(slot.RoomsWithInvigilators, &model.RoomWithInvigilator{
			Name:         name,
			MaxDuration:  maxDuration,
			StudentCount: studentCount,
			RoomAndExams: roomAndExams,
			Invigilator:  invigilator,
		})
	}
	return slot, nil
}
