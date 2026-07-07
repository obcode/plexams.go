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

func (db *DB) GetInvigilatorAt(ctx context.Context, roomname string, starttime time.Time) (*model.Teacher, error) {
	invigilations, err := db.GetInvigilationsAt(ctx, roomname, starttime)
	if err != nil {
		log.Error().Err(err).Str("room", roomname).Time("starttime", starttime).Msg("cannot get invigilations")
		return nil, err
	}

	if len(invigilations) > 1 {
		log.Error().Str("room", roomname).Time("starttime", starttime).
			Interface("invigilations", invigilations).
			Msg("found more than one invigilation")
		return nil, fmt.Errorf("found more than one invigilation")
	}
	if len(invigilations) == 0 {
		return nil, nil
	}

	return db.GetTeacher(ctx, invigilations[0].InvigilatorID)
}

func (db *DB) GetInvigilationsAt(ctx context.Context, roomname string, starttime time.Time) ([]*model.Invigilation, error) {
	invigilations, err := db.getInvigilationsAt(ctx, collectionSelfInvigilations, roomname, starttime)
	if err != nil {
		log.Error().Err(err).Str("room", roomname).Time("starttime", starttime).Msg("cannot get self invigilations")
		return nil, err
	}

	other, err := db.getInvigilationsAt(ctx, collectionOtherInvigilations, roomname, starttime)
	if err != nil {
		log.Error().Err(err).Str("room", roomname).Time("starttime", starttime).Msg("cannot get other invigilations")
		return nil, err
	}

	return append(invigilations, other...), nil
}

func (db *DB) getInvigilationsAt(ctx context.Context, collectionName, roomname string, starttime time.Time) ([]*model.Invigilation, error) {
	collection := db.getCollectionSemester(collectionName)

	var filter primitive.M
	if roomname == "reserve" {
		filter = bson.M{
			"$and": []bson.M{
				{"roomname": nil},
				{"isreserve": true},
				{"starttime": starttime},
			},
		}
	} else {
		filter = bson.M{
			"$and": []bson.M{
				{"roomname": roomname},
				{"isreserve": false},
				{"starttime": starttime},
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
	for _, inv := range invigilations {
		db.decorateInvigilation(inv)
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
	for _, inv := range invigilations {
		db.decorateInvigilation(inv)
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
	for _, inv := range invigilations {
		db.decorateInvigilation(inv)
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
	for _, inv := range invigilations {
		db.decorateInvigilation(inv)
	}

	return invigilations, nil
}

func (db *DB) AddInvigilationAt(ctx context.Context, room string, starttime time.Time, invigilatorID int) error {
	collection := db.getCollectionSemester(collectionOtherInvigilations)

	var filter primitive.M
	// default is reserve: its duration is the slot's longest invigilation (not
	// the credited 60 min, which is applied in PrepareInvigilationTodos).
	duration := db.getMaxDurationAt(ctx, starttime)
	isReserve := true
	var roomname *string
	if room == "reserve" {
		filter = bson.M{
			"$and": []bson.M{
				{"roomname": nil},
				{"isreserve": true},
				{"starttime": starttime},
			},
		}
	} else {
		filter = bson.M{
			"$and": []bson.M{
				{"roomname": room},
				{"isreserve": false},
				{"starttime": starttime},
			},
		}
		duration = db.getMaxDurationForRoomAt(ctx, room, starttime)
		isReserve = false
		roomname = &room
	}

	opts := options.Replace().SetUpsert(true)

	_, err := collection.ReplaceOne(ctx, filter,
		model.Invigilation{
			Starttime:          &starttime,
			RoomName:           roomname,
			Duration:           duration,
			InvigilatorID:      invigilatorID,
			IsReserve:          isReserve,
			IsSelfInvigilation: false,
		},
		opts)

	if err != nil {
		log.Error().Err(err).Str("room", room).Time("starttime", starttime).Int("invigilator-id", invigilatorID).
			Msg("cannot add  invigilation")
		return err
	}

	return nil
}

// SetInvigilationPrePlannedAt sets the prePlanned flag on the invigilation for a
// room (roomName != nil) or the reserve (roomName == nil) at a start time in the
// invigilations_other collection.
func (db *DB) SetInvigilationPrePlannedAt(ctx context.Context, starttime time.Time, roomName *string, prePlanned bool) error {
	collection := db.getCollectionSemester(collectionOtherInvigilations)
	filter := bson.M{
		"roomname":  roomName,
		"isreserve": roomName == nil,
		"starttime": starttime,
	}
	res, err := collection.UpdateOne(ctx, filter, bson.M{"$set": bson.M{"preplanned": prePlanned}})
	if err != nil {
		log.Error().Err(err).Time("starttime", starttime).Msg("cannot set prePlanned on invigilation")
		return err
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("no invigilation found to mark as pre-planned at %s", starttime.Format("02.01. 15:04"))
	}
	return nil
}

func (db *DB) getMaxDurationForRoomAt(ctx context.Context, roomname string, starttime time.Time) int {
	maxDuration := 0

	examsInSlot, _ := db.ExamsAt(ctx, starttime)
	for _, exam := range examsInSlot {
		for _, room := range exam.PlannedRooms {
			if roomname == room.RoomName && maxDuration < room.Duration {
				maxDuration = room.Duration
			}
		}
	}

	return maxDuration
}

// getMaxDurationAt returns the longest invigilation (room duration) across
// all rooms at the start time, used as the time block for a reserve invigilation.
func (db *DB) getMaxDurationAt(ctx context.Context, starttime time.Time) int {
	maxDuration := 0

	examsInSlot, _ := db.ExamsAt(ctx, starttime)
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
	// The absolute start time is the source of truth; the caller sets it.
	if prePlannedInvigilation.Starttime == nil {
		return false, fmt.Errorf("pre-planned invigilation has no start time")
	}
	starttime := *prePlannedInvigilation.Starttime
	// Only one invigilator per room (or reserve) at a time: delete any existing
	// document with the same start time and room before inserting the new one.
	filter := bson.M{
		"starttime": starttime,
		"roomname":  prePlannedInvigilation.RoomName,
	}
	_, err := collection.DeleteOne(ctx, filter)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionInvigilationsPrePlanned).
			Time("starttime", starttime).
			Interface("roomname", prePlannedInvigilation.RoomName).
			Msg("cannot delete existing pre planned invigilation")
		return false, err
	}
	_, err = collection.InsertOne(ctx, prePlannedInvigilation)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionInvigilationsPrePlanned).
			Time("starttime", starttime).
			Int("invigilator-id", prePlannedInvigilation.InvigilatorID).
			Msg("cannot insert pre planned invigilation")
		return false, err
	}
	return true, nil
}

// ResetGeneratedInvigilations drops the generated invigilations
// (invigilations_other). The pre-planning (invigilations_pre_planned) and the
// self-invigilations (invigilations_self) are not touched.
func (db *DB) ResetGeneratedInvigilations(ctx context.Context) error {
	collection := db.getCollectionSemester(collectionOtherInvigilations)
	if err := collection.Drop(ctx); err != nil {
		log.Error().Err(err).Str("collection", collectionOtherInvigilations).Msg("cannot drop generated invigilations")
		return err
	}
	return nil
}

// RemovePrePlannedInvigilationAt deletes a pre-planned invigilation (key:
// starttime/roomName; roomName nil = the reserve). It reports whether a document
// was actually removed.
func (db *DB) RemovePrePlannedInvigilationAt(ctx context.Context, starttime time.Time, roomName *string) (bool, error) {
	collection := db.getCollectionSemester(collectionInvigilationsPrePlanned)
	filter := bson.M{
		"starttime": starttime,
		"roomname":  roomName,
	}
	res, err := collection.DeleteOne(ctx, filter)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionInvigilationsPrePlanned).
			Time("starttime", starttime).Interface("roomname", roomName).
			Msg("cannot delete pre planned invigilation")
		return false, err
	}
	return res.DeletedCount > 0, nil
}
