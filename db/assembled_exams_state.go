package db

import (
	"context"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// SetAssembledExamsDirty upserts the (single) generated-exams state document:
// dirty=true when an input changed, dirty=false right after a (re)generation.
func (db *DB) SetAssembledExamsDirty(ctx context.Context, dirty bool, reason string, t time.Time) error {
	collection := db.getCollectionSemester(collectionAssembledExamsState)

	tt := t
	state := &model.AssembledExamsState{
		Dirty:     dirty,
		ChangedAt: &tt,
	}
	if reason != "" {
		state.Reason = &reason
	}

	_, err := collection.ReplaceOne(ctx, bson.M{}, state, options.Replace().SetUpsert(true))
	if err != nil {
		log.Error().Err(err).Bool("dirty", dirty).Msg("cannot set generated-exams state")
		return err
	}
	return nil
}

// GetAssembledExamsState returns the generated-exams state; a missing document means
// nothing has been generated yet and is reported as not dirty.
func (db *DB) GetAssembledExamsState(ctx context.Context) (*model.AssembledExamsState, error) {
	collection := db.getCollectionSemester(collectionAssembledExamsState)

	var state model.AssembledExamsState
	err := collection.FindOne(ctx, bson.M{}).Decode(&state)
	if err == mongo.ErrNoDocuments {
		return &model.AssembledExamsState{Dirty: false}, nil
	}
	if err != nil {
		log.Error().Err(err).Msg("cannot get generated-exams state")
		return nil, err
	}
	return &state, nil
}
