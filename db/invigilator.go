package db

import (
	"context"
	"fmt"

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

func (db *DB) GetInvigilatorForRoom(ctx context.Context, name string, day, time int) (*model.Teacher, error) {
	self, err := db.getInvigilatorForRoom(ctx, collectionSelfInvigilations, name, day, time)
	if err != nil {
		return nil, err
	}
	if self != nil {
		return self, nil
	}
	other, err := db.getInvigilatorForRoom(ctx, collectionOtherInvigilations, name, day, time)
	if err != nil {
		return nil, err
	}
	return other, nil
}

func (db *DB) getInvigilatorForRoom(ctx context.Context, collectionName, name string, day, time int) (*model.Teacher, error) {
	collection := db.getCollectionSemester(collectionName)

	filter := bson.M{
		"$and": []bson.M{
			{"roomname": name},
			{"slot.daynumber": day},
			{"slot.slotnumber": time},
		},
	}
	var invigilation model.Invigilation
	res := collection.FindOne(ctx, filter)
	if res.Err() == mongo.ErrNoDocuments {
		log.Debug().Err(res.Err()).Str("room", name).
			Int("day", day).Int("slot", time).Msg("no invigilation found")
		return nil, nil
	}
	if res.Err() != nil {
		log.Debug().Err(res.Err()).Str("room", name).
			Int("day", day).Int("slot", time).Msg("error while trying to find invigilation")
		return nil, res.Err()
	}
	err := res.Decode(&invigilation)
	if err != nil {
		log.Debug().Err(res.Err()).Str("room", name).
			Int("day", day).Int("slot", time).Msg("cannot decode invigilation")
		return nil, err
	}

	return db.GetTeacher(ctx, invigilation.InvigilatorID)
}

func (db *DB) CacheInvigilatorTodos(ctx context.Context, todos *model.InvigilationTodos) error {
	collection := db.Client.Database(db.databaseName).Collection(collectionInvigilatorTodos)

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
		err := fmt.Errorf("found more than one todo")
		log.Error().Err(err).Msg("cannot decode invigilator todos")
		return nil, err
	}

	return todos[0], nil
}
