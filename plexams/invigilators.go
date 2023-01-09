package plexams

import (
	"context"
	"fmt"
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
				ExamDateTimes:          examDateTimes,
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

func (p *Plexams) PrepareSelfInvigilation() error {
	ctx := context.Background()
	invigilators, err := p.InvigilatorsWithReq(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get invigilators")
		return err
	}

	invigilatorMap := make(map[int]*model.Invigilator)
	for _, invigilator := range invigilators {
		invigilatorMap[invigilator.Teacher.ID] = invigilator
	}

	// invigilations := make([]*model.Invigilation, 0)
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
				// invigilation := model.Invigilation{
				// 	RoomName:      &roomNames.ToSlice()[0],
				// 	InvigilatorID: examer,
				// 	Slot:          slot,
				// }
				// invigilations = append(invigilations, &invigilation)
			}
		}

	}
	return nil
}
