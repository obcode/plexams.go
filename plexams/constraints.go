package plexams

import (
	"context"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) NotPlannedByMe(ctx context.Context, ancode int) (bool, error) {
	return p.dbClient.NotPlannedByMe(ctx, ancode)
}

func (p *Plexams) ExcludeDays(ctx context.Context, ancode int, dayStrings []string) (bool, error) {
	days := make([]*time.Time, 0, len(dayStrings))
	for _, dayStr := range dayStrings {
		dayUTC, err := time.Parse("2006-01-02", dayStr)
		if err != nil {
			log.Error().Err(err).Str("date", dayStr).Msg("cannot parse date")
		}
		day := time.Date(dayUTC.Year(), dayUTC.Month(), dayUTC.Day(), 0, 0, 0, 0, time.Local)
		days = append(days, &day)
	}

	return p.dbClient.ExcludeDays(ctx, ancode, days)
}

func (p *Plexams) SameSlot(ctx context.Context, ancodeInput int, ancodesInput []int) (bool, error) {
	// FIXME: Does not work on updates.
	allAncodes := append(ancodesInput, ancodeInput)

	allOK := true

	for _, ancode := range allAncodes {
		ancodes := make([]int, 0, len(ancodesInput))
		for _, ac := range allAncodes {
			if ac != ancode {
				ancodes = append(ancodes, ac)
			}
		}
		log.Debug().Int("ancode", ancode).Interface("ancodes", ancodes).
			Msg("inerting")
		ok, err := p.dbClient.SameSlot(ctx, ancode, ancodes)
		if err != nil {
			log.Error().Err(err).Int("ancode", ancode).Interface("ancodes", ancodes).
				Msg("cannot set same ancode")
			allOK = allOK && ok
		}
	}

	return allOK, nil
}

func (p *Plexams) ExahmRooms(ctx context.Context, ancode int) (bool, error) {
	return p.dbClient.ExahmRooms(ctx, ancode)
}

func (p *Plexams) PlacesWithSockets(ctx context.Context, ancode int) (bool, error) {
	return p.dbClient.PlacesWithSockets(ctx, ancode)
}

func (p *Plexams) ConstraintForAncode(ctx context.Context, ancode int) (*model.Constraints, error) {
	return p.dbClient.GetConstraintsForAncode(ctx, ancode)
}

func (p *Plexams) ZpaExamsToPlanWithConstraints(ctx context.Context) ([]*model.ZPAExamWithConstraints, error) {
	exams, err := p.GetZpaExamsToPlan(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get zpa exams to plan")
	}

	examsWithConstraints := make([]*model.ZPAExamWithConstraints, 0, len(exams))
	for _, exam := range exams {
		constraint, err := p.ConstraintForAncode(ctx, exam.AnCode)
		if err != nil {
			log.Error().Err(err).Int("ancode", exam.AnCode).Msg("cannot get constraint")
		}
		examsWithConstraints = append(examsWithConstraints, &model.ZPAExamWithConstraints{
			ZpaExam:     exam,
			Constraints: constraint,
		})
	}

	return examsWithConstraints, nil
}
