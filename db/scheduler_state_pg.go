package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/rs/zerolog/log"
)

// GetSchedulerState returns the persisted scheduler state, or nil when none is
// stored yet (fresh deploy → no catch-up).
//
// last_finished and last_status are null between the first TouchSchedulerFire and
// the first completed run: the anchor is written before the run executes, so the
// outcome columns have nothing to say yet. SchedulerState expresses that as the
// zero time and the empty string, exactly as the Mongo decoder did for a document
// that carried only the three touched fields.
func (db *PG) GetSchedulerState(ctx context.Context) (*SchedulerState, error) {
	row, err := db.q(ctx).GetSchedulerState(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		log.Error().Err(err).Msg("cannot get scheduler state")
		return nil, err
	}

	state := &SchedulerState{
		LastFireAt:   row.LastFireAt,
		LastTrigger:  row.LastTrigger,
		TotalChanges: row.TotalChanges,
	}
	if row.LastFinished != nil {
		state.LastFinished = *row.LastFinished
	}
	if row.LastStatus != nil {
		state.LastStatus = *row.LastStatus
	}
	if row.SemesterID != nil {
		state.Semester = *row.SemesterID
	}
	return state, nil
}

// TouchSchedulerFire records the start of a fire: it sets the catch-up anchor and
// the trigger/semester before the run executes, so a crash or several restarts
// within the same day do not re-trigger the catch-up against a stale anchor. It
// leaves the previous run's outcome fields intact.
func (db *PG) TouchSchedulerFire(ctx context.Context, at time.Time, trigger, semester string) error {
	err := db.q(ctx).TouchSchedulerFire(ctx, sqlc.TouchSchedulerFireParams{
		LastFireAt:  at,
		LastTrigger: trigger,
		SemesterID:  &semester,
	})
	if err != nil {
		log.Error().Err(err).Msg("cannot record scheduler fire")
	}
	return err
}

// SaveSchedulerState overwrites the scheduler state with the outcome of a
// finished run.
func (db *PG) SaveSchedulerState(ctx context.Context, state *SchedulerState) error {
	params := sqlc.SaveSchedulerStateParams{
		LastFireAt:   state.LastFireAt,
		LastTrigger:  state.LastTrigger,
		SemesterID:   &state.Semester,
		TotalChanges: state.TotalChanges,
	}
	// The zero values mean "no outcome yet" and belong in the columns as null --
	// '' is not one of the statuses the check allows.
	if !state.LastFinished.IsZero() {
		params.LastFinished = &state.LastFinished
	}
	if state.LastStatus != "" {
		params.LastStatus = &state.LastStatus
	}

	if err := db.q(ctx).SaveSchedulerState(ctx, params); err != nil {
		log.Error().Err(err).Msg("cannot save scheduler state")
		return err
	}
	return nil
}
