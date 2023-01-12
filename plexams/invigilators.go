package plexams

import (
	"context"
	"fmt"
	"math"
	"time"

	set "github.com/deckarep/golang-set/v2"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func (p *Plexams) InvigilatorsWithReq(ctx context.Context) ([]*model.Invigilator, error) {
	teachers, err := p.GetInvigilators(ctx)
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
			if invigilatorConstraints != nil {
				excludedDates := viper.GetStringSlice(fmt.Sprintf("invigilatorConstraints.%d.excludedDates", teacher.ID))
				if len(excludedDates) > 0 {
					log.Debug().Interface("excludedDates", excludedDates).Str("name", teacher.Shortname).Msg("found in config")
					reqs.ExcludedDates = append(reqs.ExcludedDates, excludedDates...)
				}
			}

			excludedDates := make([]*time.Time, 0, len(reqs.ExcludedDates))
			for _, day := range reqs.ExcludedDates {
				t, err := time.Parse("02.01.06", day)
				if err != nil {
					log.Error().Err(err).Str("day", day).Msg("cannot parse date")
				} else {
					excludedDates = append(excludedDates, &t)
				}
			}

			examDateTimes := make([]*time.Time, 0)
			exams, err := p.dbClient.PlannedExamsByMainExamer(ctx, teacher.ID)
			if err != nil {
				log.Error().Err(err).Str("name", teacher.Shortname).Msg("cannit get exams by main examer")
			} else {
				for _, exam := range exams {
					examDateTimes = append(examDateTimes, &exam.Slot.Starttime)
				}
			}

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
	days := make([]int, 0, len(dates))
	for _, date := range dates {
		for _, day := range p.semesterConfig.Days {
			if day.Date.Month() == date.Month() && day.Date.Day() == date.Day() {
				days = append(days, day.Number)
			}
		}
	}
	return days
}

func (p *Plexams) InvigilatorTodos(ctx context.Context) (*model.InvigilatorTodos, error) {
	selfInvigilations, err := p.GetSelfInvigilations(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get self invigilations")
	}

	todos := model.InvigilatorTodos{
		SumExamRooms:          0,
		SumReserve:            0,
		SumOtherContributions: 0,
		InvigilatorCount:      0,
		TodoPerInvigilator:    0,
	}

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

	for _, invigilator := range reqs {
		todos.SumOtherContributions += invigilator.Requirements.OralExamsContribution +
			invigilator.Requirements.LiveCodingContribution +
			invigilator.Requirements.MasterContribution
	}

	todos.InvigilatorCount = len(reqs)
	adjustedInvigilatorCount := 0.0

	for _, invigilator := range reqs {
		count := 1.0 * invigilator.Requirements.PartTime

		if invigilator.Requirements.OvertimeThisSemester != 0 {
			count *= invigilator.Requirements.OvertimeThisSemester
		}

		if invigilator.Requirements.FreeSemester == 0.5 {
			count *= 0.5
		}

		// TODO: Move me to InvigilatorsWithReq
		if invigilator.Requirements.FreeSemester == 1.0 ||
			invigilator.Requirements.OvertimeLastSemester != 0 && invigilator.Requirements.FreeSemester == 0.5 {
			count = 0.0
		}

		log.Debug().Str("name", invigilator.Teacher.Shortname).Float64("faktor", count).
			Msg("Faktor für Aufsichten")

		adjustedInvigilatorCount += count
	}

	todos.TodoPerInvigilator = int(math.Ceil(float64(todos.SumExamRooms+todos.SumReserve+todos.SumOtherContributions) / adjustedInvigilatorCount))

	return &todos, nil
}

func (p *Plexams) PrepareSelfInvigilation() error {
	return nil
}

func (p *Plexams) GetSelfInvigilations(ctx context.Context) ([]*model.Invigilation, error) {
	invigilators, err := p.InvigilatorsWithReq(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get invigilators")
		return nil, err
	}

	invigilatorMap := make(map[int]*model.Invigilator)
	for _, invigilator := range invigilators {
		invigilatorMap[invigilator.Teacher.ID] = invigilator
	}

	invigilations := make([]*model.Invigilation, 0)
	for _, slot := range p.semesterConfig.Slots {
		examsInSlot, err := p.ExamsInSlot(ctx, slot.DayNumber, slot.SlotNumber)
		if err != nil {
			log.Error().Err(err).Int("day", slot.DayNumber).Int("time", slot.SlotNumber).
				Msg("cannot get exams in slot")
			continue
		}
		examerWithExams := make(map[int][]*model.ExamInPlan)
		for _, exam := range examsInSlot {
			if _, ok := invigilatorMap[exam.Exam.ZpaExam.MainExamerID]; !ok {
				log.Debug().Str("name", exam.Exam.ZpaExam.MainExamer).Msg("ist keine Aufsicht")
				continue
			}
			exams, ok := examerWithExams[exam.Exam.ZpaExam.MainExamerID]
			if !ok {
				examerWithExams[exam.Exam.ZpaExam.MainExamerID] = []*model.ExamInPlan{exam}
			} else {
				examerWithExams[exam.Exam.ZpaExam.MainExamerID] = append(exams, exam)
			}
		}

		for examer, exams := range examerWithExams {
			roomNames := set.NewSet[string]()
			for _, exam := range exams {
				rooms, err := p.dbClient.RoomsForAncode(ctx, exam.Exam.Ancode)

				if err != nil {
					log.Error().Err(err).Int("ancode", exam.Exam.Ancode).Msg("cannot get rooms for ancode")
				} else {
					for _, room := range rooms {
						roomNames.Add(room.RoomName)
					}
				}
			}

			if roomNames.Cardinality() == 1 {
				log.Debug().Int("examerid", examer).Interface("room", roomNames).Interface("slot", slot).
					Msg("found self invigilation")
				invigilation := model.Invigilation{
					RoomName:      &roomNames.ToSlice()[0],
					InvigilatorID: examer,
					Slot:          slot,
				}
				invigilations = append(invigilations, &invigilation)
			}
		}

	}
	return invigilations, nil
}
