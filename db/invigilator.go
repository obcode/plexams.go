package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/zpa"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
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

// invigilatorTodosID is the fixed _id of the single todos document. Writing against
// it makes the cache write idempotent: concurrent writers overwrite the same document
// instead of racing a drop against an insert.
const invigilatorTodosID = "todos"

func (db *DB) CacheInvigilatorTodos(ctx context.Context, todos *model.InvigilationTodos) error {
	collection := db.Client.Database(db.databaseName).Collection(collectionInvigilatorTodos)

	// GetInvigilationTodos re-caches on every read, so parallel validation
	// subscriptions call this concurrently. A drop followed by an insert can
	// interleave (A drop, B drop, A insert, B insert) and leave two documents behind,
	// which then breaks every reader. Replacing one document under a fixed _id cannot
	// interleave that way, so no lock is needed.
	_, err := collection.ReplaceOne(ctx,
		bson.M{"_id": invigilatorTodosID}, todos, options.Replace().SetUpsert(true))
	if err != nil {
		log.Error().Err(err).Msg("cannot cache invigilator todos")
		return err
	}

	// Heal documents written before the fixed _id (and any left over from an earlier
	// interleaved write).
	if _, err := collection.DeleteMany(ctx,
		bson.M{"_id": bson.M{"$ne": invigilatorTodosID}}); err != nil {
		log.Warn().Err(err).Msg("cannot remove stale invigilator todos documents")
	}

	return nil
}

func (db *DB) GetInvigilationTodos(ctx context.Context) (*model.InvigilationTodos, error) {
	collection := db.Client.Database(db.databaseName).Collection(collectionInvigilatorTodos)

	var todos model.InvigilationTodos
	err := collection.FindOne(ctx, bson.M{"_id": invigilatorTodosID}).Decode(&todos)
	if err == mongo.ErrNoDocuments {
		// Written before the fixed _id: fall back to any document. The next
		// CacheInvigilatorTodos moves the collection to the canonical one.
		err = collection.FindOne(ctx, bson.M{}).Decode(&todos)
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
	}
	if err != nil {
		log.Error().Err(err).Msg("cannot find invigilator todos")
		return nil, err
	}

	return &todos, nil
}
