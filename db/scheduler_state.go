package db

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// SchedulerState is the persisted state of the nightly auto-sync scheduler. It is a
// server-wide singleton (stored in the global "plexams" database, not per-semester) because
// the scheduler is a server-wide singleton. LastFireAt is the catch-up anchor: it records
// when a run was last *attempted* (not when it last succeeded), so a failed or skipped night
// still advances it and only a genuinely missed fire (process down across the scheduled time)
// triggers a make-up run on the next start.
type SchedulerState struct {
	LastFireAt   time.Time `bson:"lastFireAt"`   // catch-up anchor (attempt time)
	LastFinished time.Time `bson:"lastFinished"` // when the last run finished
	LastStatus   string    `bson:"lastStatus"`   // ok|errors|skipped|panic
	LastTrigger  string    `bson:"lastTrigger"`  // nightly|catchup|manual
	Semester     string    `bson:"semester"`     // which workspace the last run synced
	TotalChanges int       `bson:"totalChanges"` // changes found in the last run
}

// GetSchedulerState returns the persisted scheduler state, or nil when none is stored yet
// (fresh deploy → no catch-up).
func (db *DB) GetSchedulerState(ctx context.Context) (*SchedulerState, error) {
	collection := db.Client.Database("plexams").Collection(collectionSchedulerState)
	var state SchedulerState
	err := collection.FindOne(ctx, bson.M{}).Decode(&state)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		log.Error().Err(err).Msg("cannot get scheduler state")
		return nil, err
	}
	return &state, nil
}

// TouchSchedulerFire records the start of a fire: it sets the catch-up anchor (LastFireAt) and
// the trigger/semester *before* the run executes, so a crash or several restarts within the
// same day do not re-trigger the catch-up against a stale anchor. It only $sets those fields,
// leaving the previous run's outcome fields intact until SaveSchedulerState overwrites them.
func (db *DB) TouchSchedulerFire(ctx context.Context, at time.Time, trigger, semester string) error {
	collection := db.Client.Database("plexams").Collection(collectionSchedulerState)
	_, err := collection.UpdateOne(ctx, bson.M{},
		bson.M{"$set": bson.M{
			"lastFireAt":  at,
			"lastTrigger": trigger,
			"semester":    semester,
		}},
		options.Update().SetUpsert(true))
	if err != nil {
		log.Error().Err(err).Msg("cannot record scheduler fire")
	}
	return err
}

// SaveSchedulerState overwrites the scheduler state with the outcome of a finished run.
func (db *DB) SaveSchedulerState(ctx context.Context, state *SchedulerState) error {
	collection := db.Client.Database("plexams").Collection(collectionSchedulerState)
	_, err := collection.ReplaceOne(ctx, bson.M{}, state, options.Replace().SetUpsert(true))
	if err != nil {
		log.Error().Err(err).Msg("cannot save scheduler state")
	}
	return err
}
