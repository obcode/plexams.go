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
	skipNoPlan          = "noch kein Terminplan generiert"
	skipNoRooms         = "noch keine Räume geplant"
	skipNoInvigilations = "noch keine Aufsichten eingeteilt"
)

// planGenerated reports whether the exam schedule has been generated (the
// examScheduleGenerated milestone). Plan entries may already exist earlier — e.g. the
// EXaHM/SEB pre-planning phase fixes a few exams into slots before the full schedule is
// generated — so the conflict/constraint validators gate on this milestone rather than
// on "any plan entry exists", which would let them run prematurely.
func (p *Plexams) planGenerated(ctx context.Context) (bool, error) {
	setKeys, err := p.dbClient.PlanningConditionsSet(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get planning conditions")
		return false, err
	}
	for _, key := range setKeys {
		if key == condExamScheduleGenerated {
			return true, nil
		}
	}
	return false, nil
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
