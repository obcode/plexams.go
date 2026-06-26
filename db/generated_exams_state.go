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

// SetGeneratedExamsDirty upserts the (single) generated-exams state document:
// dirty=true when an input changed, dirty=false right after a (re)generation.
func (db *DB) SetGeneratedExamsDirty(ctx context.Context, dirty bool, reason string, t time.Time) error {
	collection := db.getCollectionSemester(collectionGeneratedExamsState)

	tt := t
	state := &model.GeneratedExamsState{
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

// GetGeneratedExamsState returns the generated-exams state; a missing document means
// nothing has been generated yet and is reported as not dirty.
func (db *DB) GetGeneratedExamsState(ctx context.Context) (*model.GeneratedExamsState, error) {
	collection := db.getCollectionSemester(collectionGeneratedExamsState)

	var state model.GeneratedExamsState
	err := collection.FindOne(ctx, bson.M{}).Decode(&state)
	if err == mongo.ErrNoDocuments {
		return &model.GeneratedExamsState{Dirty: false}, nil
	}
	if err != nil {
		log.Error().Err(err).Msg("cannot get generated-exams state")
		return nil, err
	}
	return &state, nil
}
