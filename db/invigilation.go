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
		log.Error().Err(err).Str("collection", collectionName).Msg("Cannot decode to invigilations")
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
	defer cur.Close(ctx) //nolint:errcheck

	invigilations := make([]*model.Invigilation, 0)
	err = cur.All(ctx, &invigilations)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionSelfInvigilations).Msg("Cannot decode to rooms")
		return nil, err
	}

	return invigilations, nil
}

func (db *DB) GetAllInvigilations(ctx context.Context) ([]*model.Invigilation, error) {
	selfInvigilations, err := db.GetSelfInvigilations(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get self invigilations")
		return nil, err
	}
	otherInvigilations, err := db.GetOtherInvigilations(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get other invigilations")
		return nil, err
	}

	return append(selfInvigilations, otherInvigilations...), nil
}

func (db *DB) GetSelfInvigilations(ctx context.Context) ([]*model.Invigilation, error) {
	collection := db.getCollectionSemester(collectionSelfInvigilations)

	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Msg("cannot get invgilations")
		return nil, err
	}
	defer cur.Close(ctx) //nolint:errcheck

	invigilations := make([]*model.Invigilation, 0)
	err = cur.All(ctx, &invigilations)
	if err != nil {
		log.Error().Err(err).Msg("cannot get decode invgilations")
		return nil, err
	}

	return invigilations, nil
}

func (db *DB) GetOtherInvigilations(ctx context.Context) ([]*model.Invigilation, error) {
	collection := db.getCollectionSemester(collectionOtherInvigilations)

	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Msg("cannot get invgilations")
		return nil, err
	}
	defer cur.Close(ctx) //nolint:errcheck

	invigilations := make([]*model.Invigilation, 0)
	err = cur.All(ctx, &invigilations)
	if err != nil {
		log.Error().Err(err).Msg("cannot get decode invgilations")
		return nil, err
	}

	return invigilations, nil
}

func (db *DB) AddInvigilation(ctx context.Context, room string, day, slot, invigilatorID int) error {
	collection := db.getCollectionSemester(collectionOtherInvigilations)

	var filter primitive.M
	// default is reserve: its duration is the slot's longest invigilation (not
	// the credited 60 min, which is applied in PrepareInvigilationTodos).
	duration := db.getMaxDurationInSlot(ctx, day, slot)
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
				{"roomname": room},
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

// SetInvigilationPrePlanned sets the prePlanned flag on the invigilation for a
// room (roomName != nil) or the reserve (roomName == nil) in a slot in the
// invigilations_other collection.
func (db *DB) SetInvigilationPrePlanned(ctx context.Context, day, slot int, roomName *string, prePlanned bool) error {
	collection := db.getCollectionSemester(collectionOtherInvigilations)
	filter := bson.M{
		"roomname":        roomName,
		"isreserve":       roomName == nil,
		"slot.daynumber":  day,
		"slot.slotnumber": slot,
	}
	res, err := collection.UpdateOne(ctx, filter, bson.M{"$set": bson.M{"preplanned": prePlanned}})
	if err != nil {
		log.Error().Err(err).Int("day", day).Int("slot", slot).Msg("cannot set prePlanned on invigilation")
		return err
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("no invigilation found to mark as pre-planned in slot (%d,%d)", day, slot)
	}
	return nil
}

func (db *DB) getMaxDurationForRoomInSlot(ctx context.Context, roomname string, day, slot int) int {
	maxDuration := 0

	examsInSlot, _ := db.ExamsInSlot(ctx, day, slot)
	for _, exam := range examsInSlot {
		for _, room := range exam.PlannedRooms {
			if roomname == room.RoomName && maxDuration < room.Duration {
				maxDuration = room.Duration
			}
		}
	}

	return maxDuration
}

// getMaxDurationInSlot returns the longest invigilation (room duration) across
// all rooms in the slot, used as the time block for a reserve invigilation.
func (db *DB) getMaxDurationInSlot(ctx context.Context, day, slot int) int {
	maxDuration := 0

	examsInSlot, _ := db.ExamsInSlot(ctx, day, slot)
	for _, exam := range examsInSlot {
		for _, room := range exam.PlannedRooms {
			if maxDuration < room.Duration {
				maxDuration = room.Duration
			}
		}
	}

	return maxDuration
}

func (db *DB) PrePlannedInvigilations(ctx context.Context) ([]*model.PrePlannedInvigilation, error) {
	collection := db.getCollectionSemester(collectionInvigilationsPrePlanned)

	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Str("collection", collectionInvigilationsPrePlanned).Msg("MongoDB Find")
		return nil, err
	}
	defer cur.Close(ctx) //nolint:errcheck

	invigilations := make([]*model.PrePlannedInvigilation, 0)
	err = cur.All(ctx, &invigilations)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionInvigilationsPrePlanned).Msg("Cannot decode to pre planned invigilations")
		return nil, err
	}

	return invigilations, nil
}

func (db *DB) PrePlannedInvigilationsForInvigilator(ctx context.Context, invigilatorID int) ([]*model.PrePlannedInvigilation, error) {
	collection := db.getCollectionSemester(collectionInvigilationsPrePlanned)

	cur, err := collection.Find(ctx, bson.M{"invigilatorid": invigilatorID})
	if err != nil {
		log.Error().Err(err).Str("collection", collectionInvigilationsPrePlanned).Msg("MongoDB Find")
		return nil, err
	}
	defer cur.Close(ctx) //nolint:errcheck

	invigilations := make([]*model.PrePlannedInvigilation, 0)
	err = cur.All(ctx, &invigilations)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionInvigilationsPrePlanned).Msg("Cannot decode to pre planned invigilations")
		return nil, err
	}

	return invigilations, nil
}

func (db *DB) AddPrePlannedInvigilation(ctx context.Context, prePlannedInvigilation *model.PrePlannedInvigilation) (bool, error) {
	collection := db.getCollectionSemester(collectionInvigilationsPrePlanned)
	// Only one invigilator per room (or reserve) in a slot: delete any existing
	// document with the same day, slot, and room before inserting the new one.
	filter := bson.M{
		"day":      prePlannedInvigilation.Day,
		"slot":     prePlannedInvigilation.Slot,
		"roomname": prePlannedInvigilation.RoomName,
	}
	_, err := collection.DeleteOne(ctx, filter)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionInvigilationsPrePlanned).
			Int("day", prePlannedInvigilation.Day).Int("slot", prePlannedInvigilation.Slot).
			Interface("roomname", prePlannedInvigilation.RoomName).
			Msg("cannot delete existing pre planned invigilation")
		return false, err
	}
	_, err = collection.InsertOne(ctx, prePlannedInvigilation)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionInvigilationsPrePlanned).
			Int("day", prePlannedInvigilation.Day).Int("slot", prePlannedInvigilation.Slot).
			Int("invigilator-id", prePlannedInvigilation.InvigilatorID).
			Msg("cannot insert pre planned invigilation")
		return false, err
	}
	return true, nil
}

// RemovePrePlannedInvigilation deletes a pre-planned invigilation (key:
// day/slot/roomName; roomName nil = the reserve). It reports whether a document
// was actually removed.
func (db *DB) RemovePrePlannedInvigilation(ctx context.Context, day, slot int, roomName *string) (bool, error) {
	collection := db.getCollectionSemester(collectionInvigilationsPrePlanned)
	filter := bson.M{
		"day":      day,
		"slot":     slot,
		"roomname": roomName,
	}
	res, err := collection.DeleteOne(ctx, filter)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionInvigilationsPrePlanned).
			Int("day", day).Int("slot", slot).Interface("roomname", roomName).
			Msg("cannot delete pre planned invigilation")
		return false, err
	}
	return res.DeletedCount > 0, nil
}
