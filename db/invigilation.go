package db

import (
	"context"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func (db *DB) ReserveForSlot(ctx context.Context, day, time int) (*model.Teacher, error) {
	collection := db.getCollectionSemester(collectionOtherInvigilations)

	filter := bson.M{
		"$and": []bson.M{
			{"isreserve": true},
			{"slot.daynumber": day},
			{"slot.slotnumber": time},
		},
	}
	res := collection.FindOne(ctx, filter)
	if res.Err() == mongo.ErrNoDocuments {
		return nil, nil
	}
	if res.Err() != nil {
		return nil, res.Err()
	}
	var invigilation model.Invigilation
	err := res.Decode(&invigilation)
	if err != nil {
		log.Error().Err(err).Msg("cannot decode")
		return nil, err
	}

	return db.GetTeacher(ctx, invigilation.InvigilatorID)
}

func (db *DB) InvigilationsForInvigilator(ctx context.Context, invigilatorID int) ([]*model.Invigilation, error) {
	self, err := db.invigilationsForInvigilator(ctx, collectionSelfInvigilations, invigilatorID)
	if err != nil {
		return nil, err
	}
	other, err := db.invigilationsForInvigilator(ctx, collectionOtherInvigilations, invigilatorID)
	if err != nil {
		return nil, err
	}
	return append(self, other...), nil
}

func (db *DB) invigilationsForInvigilator(ctx context.Context, collectionName string, invigilatorID int) ([]*model.Invigilation, error) {
	collection := db.getCollectionSemester(collectionName)

	cur, err := collection.Find(ctx, bson.D{{Key: "invigilatorid", Value: invigilatorID}})
	if err != nil {
		log.Error().Err(err).Str("collection", collectionSelfInvigilations).Msg("MongoDB Find")
		return nil, err
	}
	defer cur.Close(ctx)

	invigilations := make([]*model.Invigilation, 0)
	err = cur.All(ctx, &invigilations)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionSelfInvigilations).Msg("Cannot decode to rooms")
		return nil, err
	}

	return invigilations, nil
}

func (db *DB) AddReserveInvigilation(ctx context.Context, day, slot, invigilatorID int) error {
	collection := db.getCollectionSemester(collectionOtherInvigilations)

	_, err := collection.InsertOne(ctx, model.Invigilation{
		RoomName:      nil,
		Duration:      60, // FIXME: Reserve counts 60
		InvigilatorID: invigilatorID,
		Slot: &model.Slot{
			DayNumber:  day,
			SlotNumber: slot,
			Starttime:  time.Time{},
		},
		IsReserve:          true,
		IsSelfInvigilation: false,
	})

	if err != nil {
		log.Error().Err(err).Int("day", day).Int("slot", slot).Int("invigilator-id", invigilatorID).
			Msg("cannot add reserve invigilation")
		return err
	}

	return nil
}
