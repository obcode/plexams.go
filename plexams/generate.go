package plexams

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/generateplan"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) GeneratePlan(ctx context.Context) error {

	// plan only not yet planned exam groups
	allExamGroups, err := p.dbClient.ExamGroups(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get exam groups")
		return err
	}

	log.Debug().Int("count", len(allExamGroups)).Msg("found exam groups")

	examGroups := make([]*model.ExamGroup, 0, len(allExamGroups))
	alreadyPlanned := 0
	for _, examGroup := range allExamGroups {
		planEntry, err := p.dbClient.PlanEntryForExamGroup(ctx, examGroup.ExamGroupCode)
		if err != nil {
			log.Error().Err(err).Int("examgroupcode", examGroup.ExamGroupCode).Msg("cannot get plan entry for exam group code")
			return err
		}
		// set possible slots to the one already planned slot
		if planEntry != nil {
			log.Debug().Int("examgroupcode", examGroup.ExamGroupCode).Int("day", planEntry.DayNumber).Int("slot", planEntry.SlotNumber).
				Msg("exam group already planned")
			examGroup.ExamGroupInfo.PossibleSlots = []*model.Slot{{
				DayNumber:  planEntry.DayNumber,
				SlotNumber: planEntry.SlotNumber,
			}}
			alreadyPlanned++
		} else
		// if not planned and not planned by me
		if examGroup.ExamGroupInfo.NotPlannedByMe {
			log.Debug().Int("examgroupcode", examGroup.ExamGroupCode).Msg("removing exam group --- not planned (by me)")
			continue
		}
		examGroups = append(examGroups, examGroup)
	}

	log.Debug().Int("count", len(examGroups)).Int("already planned", alreadyPlanned).Msg("using exam groups")

	algorithm := generateplan.InitalizeAlgorithm(p.semesterConfig, examGroups, 500, -1095./float64(500*500+9185)+0.3494,
		generateplan.StochasticUniversal, generateplan.UniformCrossover, 0)

	plan, err := algorithm.NRuns(100)

	if err != nil {
		log.Error().Err(err).Msg("no plan generated")
		return err
	}

	for _, entry := range plan {
		planEntry, err := p.dbClient.PlanEntryForExamGroup(ctx, entry.ExamGroupCode)
		if err != nil {
			log.Error().Err(err).Int("examgroupcode", entry.ExamGroupCode).Msg("cannot get plan entry for exam group code")
			return err
		}
		// if entry is already in plan, day and slot should be the same
		if planEntry != nil {
			if planEntry.DayNumber == entry.DayNumber && planEntry.SlotNumber == entry.SlotNumber {
				log.Debug().Interface("entry", entry).Msg("already in plan")
			} else {
				log.Error().Interface("entry", entry).Interface("planentry", planEntry).Msg("already in plan in another slot")
			}

		} else {
			// TODO: push into db
			log.Debug().Interface("entry", entry).Msg("found slot for exam group")
		}
	}

	return nil
}
