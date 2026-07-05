package plexams

import (
	"context"

	"github.com/rs/zerolog/log"
)

// Data-driven preconditions for the validators. A validator that checks planned data
// (the exam schedule, the room assignment, the invigilation assignment) makes no sense
// while that data does not exist yet — running it would only produce noise. The
// validators short-circuit via validation.skip when the relevant precondition is not
// met, keyed on the actual DB contents (self-correcting: as soon as planning starts,
// the checks run automatically).

// Skip reasons (shown to the user on a skipped validation).
const (
	skipNoPlan          = "noch kein Terminplan (keine Prüfung eingeplant)"
	skipNoRooms         = "noch keine Räume geplant"
	skipNoInvigilations = "noch keine Aufsichten eingeteilt"
)

// hasPlanEntries reports whether an exam schedule exists (any plan entry).
func (p *Plexams) hasPlanEntries(ctx context.Context) (bool, error) {
	entries, err := p.dbClient.PlanEntries(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get plan entries")
		return false, err
	}
	return len(entries) > 0, nil
}

// hasPlannedRooms reports whether any room has been assigned.
func (p *Plexams) hasPlannedRooms(ctx context.Context) (bool, error) {
	rooms, err := p.dbClient.PlannedRooms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get planned rooms")
		return false, err
	}
	return len(rooms) > 0, nil
}

// hasInvigilations reports whether any invigilation has been assigned.
func (p *Plexams) hasInvigilations(ctx context.Context) (bool, error) {
	invigilations, err := p.dbClient.GetAllInvigilations(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get invigilations")
		return false, err
	}
	return len(invigilations) > 0, nil
}
