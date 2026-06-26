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

// SetStudentRegsDirty upserts the (single) student-regs state document: dirty=true
// when an input changed, dirty=false right after a (re)generation.
func (db *DB) SetStudentRegsDirty(ctx context.Context, dirty bool, reason string, t time.Time) error {
	collection := db.getCollectionSemester(collectionStudentRegsState)

	tt := t
	state := &model.StudentRegsState{
		Dirty:     dirty,
		ChangedAt: &tt,
	}
	if reason != "" {
		state.Reason = &reason
	}

	_, err := collection.ReplaceOne(ctx, bson.M{}, state, options.Replace().SetUpsert(true))
	if err != nil {
		log.Error().Err(err).Bool("dirty", dirty).Msg("cannot set student-regs state")
		return err
	}
	return nil
}

// GetStudentRegsState returns the student-regs state; a missing document means
// nothing has been generated yet and is reported as not dirty.
func (db *DB) GetStudentRegsState(ctx context.Context) (*model.StudentRegsState, error) {
	collection := db.getCollectionSemester(collectionStudentRegsState)

	var state model.StudentRegsState
	err := collection.FindOne(ctx, bson.M{}).Decode(&state)
	if err == mongo.ErrNoDocuments {
		return &model.StudentRegsState{Dirty: false}, nil
	}
	if err != nil {
		log.Error().Err(err).Msg("cannot get student-regs state")
		return nil, err
	}
	return &state, nil
}
