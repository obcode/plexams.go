package db

import (
	"context"
	"fmt"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (db *DB) GetInvigilatorInSlot(ctx context.Context, roomname string, day, slot int) (*model.Teacher, error) {
	invigilations, err := db.GetInvigilationInSlot(ctx, roomname, day, slot)
	if err != nil {
		log.Error().Err(err).Str("room", roomname).Int("day", day).Int("slot", slot).Msg("cannot get invigilations")
		return nil, err
	}

	if len(invigilations) > 1 {
		log.Error().Str("room", roomname).Int("day", day).Int("slot", slot).
			Interface("invigilations", invigilations).
			Msg("found more than one invigilation")
		return nil, fmt.Errorf("found more than one invigilation")
	}
	if len(invigilations) == 0 {
		return nil, nil
	}

	return db.GetTeacher(ctx, invigilations[0].InvigilatorID)
}

func (db *DB) GetInvigilationInSlot(ctx context.Context, roomname string, day, slot int) ([]*model.Invigilation, error) {
	invigilations, err := db.getInvigilationInSlot(ctx, collectionSelfInvigilations, roomname, day, slot)
	if err != nil {
		log.Error().Err(err).Str("room", roomname).Int("day", day).Int("slot", slot).Msg("cannot get self invigilations")
		return nil, err
	}

	other, err := db.getInvigilationInSlot(ctx, collectionOtherInvigilations, roomname, day, slot)
	if err != nil {
		log.Error().Err(err).Str("room", roomname).Int("day", day).Int("slot", slot).Msg("cannot get other invigilations")
		return nil, err
	}

	return append(invigilations, other...), nil
}

func (db *DB) getInvigilationInSlot(ctx context.Context, collectionName, roomname string, day, slot int) ([]*model.Invigilation, error) {
	collection := db.getCollectionSemester(collectionName)

	var filter primitive.M
	if roomname == "reserve" {
		filter = bson.M{
			"$and": []bson.M{
				{"roomname": nil},
				{"isreserve": true},
				{"slot.daynumber": day},
				{"slot.slotnumber": slot},
			},
		}
	} else {
		filter = bson.M{
			"$and": []bson.M{
				{"roomname": roomname},
				{"isreserve": false},
				{"slot.daynumber": day},
				{"slot.slotnumber": slot},
			},
		}
	}

	cur, err := collection.Find(ctx, filter)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionName).Msg("Cannot find")
		return nil, err
	}

	invigilations := make([]*model.Invigilation, 0)

	err = cur.All(ctx, &invigilations)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionRooms).Msg("Cannot decode to rooms for exams")
		return nil, err
	}

	return invigilations, nil
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

func (db *DB) AddInvigilation(ctx context.Context, room string, day, slot, invigilatorID int) error {
	collection := db.getCollectionSemester(collectionOtherInvigilations)

	var filter primitive.M
	// default is reserve
	duration := 60 // FIXME: Reserve counts 60
	isReserve := true
	var roomname *string
	if room == "reserve" {
		filter = bson.M{
			"$and": []bson.M{
				{"roomname": nil},
				{"isreserve": true},
				{"slot.daynumber": day},
				{"slot.slotnumber": slot},
			},
		}
	} else {
		filter = bson.M{
			"$and": []bson.M{
				{"roomname": roomname},
				{"isreserve": false},
				{"slot.daynumber": day},
				{"slot.slotnumber": slot},
			},
		}
		duration = db.getMaxDurationForRoomInSlot(ctx, room, day, slot)
		isReserve = false
		roomname = &room
	}

	opts := options.Replace().SetUpsert(true)

	_, err := collection.ReplaceOne(ctx, filter,
		model.Invigilation{
			RoomName:      roomname,
			Duration:      duration,
			InvigilatorID: invigilatorID,
			Slot: &model.Slot{
				DayNumber:  day,
				SlotNumber: slot,
				Starttime:  time.Time{},
			},
			IsReserve:          isReserve,
			IsSelfInvigilation: false,
		},
		opts)

	if err != nil {
		log.Error().Err(err).Str("room", room).Int("day", day).Int("slot", slot).Int("invigilator-id", invigilatorID).
			Msg("cannot add  invigilation")
		return err
	}

	return nil
}

func (db *DB) getMaxDurationForRoomInSlot(ctx context.Context, roomname string, day, slot int) int {
	maxDuration := 0

	plannedRooms, _ := db.RoomsPlannedInSlot(ctx, day, slot)
	for _, room := range plannedRooms {
		if roomname == room.RoomName && maxDuration < room.Duration {
			maxDuration = room.Duration
		}
	}

	return maxDuration
}
