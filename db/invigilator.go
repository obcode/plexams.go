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

func (db *DB) GetInvigilatorForRoom(ctx context.Context, name string, day, time int) (*model.Teacher, error) {
	collection := db.getCollectionSemester(collectionSelfInvigilations)

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

	log.Debug().Msg("found one...")

	return db.GetTeacher(ctx, invigilation.InvigilatorID)
}
