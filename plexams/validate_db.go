package plexams

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// TODO: all planned_rooms okay? especially after moving an exam? check room -> slot -> ancode sameslot?
func (p *Plexams) ValidateDB(reporter Reporter) (*model.ValidationReport, error) {
	ctx := context.Background()
	v := newValidation(reporter, "db", "validating data base entries")

	planEntries, err := p.dbClient.PlanEntries(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get planEntries")
	}

	v.step("validating only one plan entry per ancode")
	planEntryMap := make(map[int]*model.PlanEntry)
	for _, planEntry := range planEntries {
		otherEntry, ok := planEntryMap[planEntry.Ancode]
		if ok {
			v.errorf(ref{Ancode: ptr(planEntry.Ancode)},
				"more than one plan entry for ancode %d: %v and %v", planEntry.Ancode, otherEntry, planEntry)
		} else {
			planEntryMap[planEntry.Ancode] = planEntry
		}
	}

	return v.finish(), nil
}
