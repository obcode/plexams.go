package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/zpa"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func (db *DB) GetInvigilatorRequirements(ctx context.Context, teacherID int) (*zpa.SupervisorRequirements, error) {
	collection := db.getCollectionSemester(collectionInvigilatorRequirements)

	var req zpa.SupervisorRequirements

	filter := bson.D{{Key: "invigilatorid", Value: teacherID}}
	err := collection.FindOne(ctx, filter).Decode(&req)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		log.Error().Err(err).Int("invigilatorid", teacherID).Msg("cannot get requirements for inviglator")
		return nil, err
	}

	return &req, nil
}

// AllInvigilatorRequirements returns all stored ZPA invigilator requirements.
func (db *DB) AllInvigilatorRequirements(ctx context.Context) ([]*zpa.SupervisorRequirements, error) {
	collection := db.getCollectionSemester(collectionInvigilatorRequirements)

	cur, err := collection.Find(ctx, bson.D{})
	if err != nil {
		log.Error().Err(err).Msg("cannot get invigilator requirements")
		return nil, err
	}

	reqs := make([]*zpa.SupervisorRequirements, 0)
	if err := cur.All(ctx, &reqs); err != nil {
		log.Error().Err(err).Msg("cannot decode invigilator requirements")
		return nil, err
	}
	return reqs, nil
}

func (db *DB) CacheInvigilatorTodos(ctx context.Context, todos *model.InvigilationTodos) error {
	collection := db.Client.Database(db.databaseName).Collection(collectionInvigilatorTodos)

	// Serialize the drop+insert: GetInvigilationTodos re-caches on every read, so
	// parallel validation subscriptions call this concurrently. Without the lock
	// their drops and inserts can interleave (A drop, B drop, A insert, B insert)
	// and leave two documents behind, which then breaks every reader.
	db.todosMu.Lock()
	defer db.todosMu.Unlock()

	err := collection.Drop(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot drop invigilator todos collection")
		return err
	}
	_, err = collection.InsertOne(ctx, todos)
	if err != nil {
		log.Error().Err(err).Msg("cannot cache invigilator todos")
		return err
	}

	return err
}

func (db *DB) GetInvigilationTodos(ctx context.Context) (*model.InvigilationTodos, error) {
	collection := db.Client.Database(db.databaseName).Collection(collectionInvigilatorTodos)

	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Msg("cannot find invigilator todos")
		return nil, err
	}

	todos := make([]*model.InvigilationTodos, 0)

	err = cur.All(ctx, &todos)
	if err != nil {
		log.Error().Err(err).Msg("cannot decode invigilator todos")
		return nil, err
	}

	if len(todos) == 0 {
		return nil, nil
	}
	if len(todos) > 1 {
		// Stale duplicates from an earlier interleaved cache write. Tolerate them
		// and return the first; the caller re-caches (drop+insert) and thereby
		// heals the collection back to a single document.
		log.Warn().Int("count", len(todos)).Msg("found more than one invigilator todos document, using the first and healing on next cache")
	}

	return todos[0], nil
}
