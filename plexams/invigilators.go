package plexams

import (
	"context"
	"fmt"
	"time"

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
