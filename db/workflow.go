package db

import (
	"context"
	"errors"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var ErrNoWorkflowInitiated = errors.New("no workflow initiated")

func (db *DB) GetWorkflow(ctx context.Context) ([]*model.Step, error) {
	collNames, err := db.Client.Database(databaseName(db.semester)).ListCollectionNames(ctx, bson.D{primitive.E{
		Key:   "name",
		Value: "workflow",
	}})

	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Msg("error while list collection names for workflow")
		return nil, err
	}

	if len(collNames) == 0 {
		log.Error().Err(err).Str("semester", db.semester).Msg("no collection for workflow found")
		return nil, ErrNoWorkflowInitiated
	}

	collection := db.Client.Database(databaseName(db.semester)).Collection("workflow")

	steps := make([]*model.Step, 0)

	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("collection", "workflow").Msg("MongoDB Find")
		return steps, err
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var step model.Step

		err := cur.Decode(&step)
		if err != nil {
			log.Error().Err(err).Str("semester", db.semester).Str("collection", "workflow").Interface("cur", cur).
				Msg("Cannot decode to step")
			return steps, err
		}

		steps = append(steps, &step)

	}

	if err := cur.Err(); err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("collection", "workflow").Msg("Cursor returned error")
		return steps, err
	}

	return steps, nil
}

func (db *DB) SetWorkflow(ctx context.Context, workflow []*model.Step) error {
	collection := db.Client.Database(databaseName(db.semester)).Collection("workflow")

	workflowIntf := make([]interface{}, 0, len(workflow))

	for _, v := range workflow {
		workflowIntf = append(workflowIntf, v)
	}

	err := collection.Drop(ctx)
	if err != nil {
		return err
	}

	_, err = collection.InsertMany(ctx, workflowIntf)
	if err != nil {
		return err
	}

	return nil
}
