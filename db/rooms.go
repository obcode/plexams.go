package db

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (db *DB) RoomByName(ctx context.Context, roomName string) (*model.Room, error) {
	collection := db.Client.Database("plexams").Collection(collectionGlobalRooms)

	res := collection.FindOne(ctx, bson.M{"name": roomName})
	if res.Err() != nil {
		if res.Err() == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("cannot find room %s", roomName)
		}
		log.Error().Err(res.Err()).Str("room", roomName).Str("collection", collectionGlobalRooms).
			Msg("cannot find room")
		return nil, res.Err()
	}

	var room model.Room
	err := res.Decode(&room)
	if err != nil {
		log.Error().Err(res.Err()).Str("room", roomName).Str("collection", collectionGlobalRooms).
			Msg("cannot decode room")

		return nil, err
	}

	return &room, nil
}

func (db *DB) Rooms(ctx context.Context) ([]*model.Room, error) {
	collection := db.Client.Database("plexams").Collection(collectionGlobalRooms)

	findOptions := options.Find()
	findOptions.SetSort(bson.D{{Key: "name", Value: 1}})

	cur, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionGlobalRooms).Msg("MongoDB Find")
		return nil, err
	}
	defer cur.Close(ctx) //nolint:errcheck

	rooms := make([]*model.Room, 0)
	err = cur.All(ctx, &rooms)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionGlobalRooms).Msg("Cannot decode to rooms")
		return nil, err
	}

	return rooms, nil
}

func (db *DB) RoomsForSlots(ctx context.Context) ([]*model.RoomsForSlot, error) {
	collection := db.getCollectionSemester(collectionRoomsForSlots)
	findOptions := options.Find()
	findOptions.SetSort(bson.D{{Key: "day", Value: 1}, {Key: "slot", Value: 1}})
	cur, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		log.Error().Err(err).Str("collectionName", collectionRoomsForSlots).
			Msg("cannot find rooms for slots")
		return nil, err
	}
	defer cur.Close(ctx) //nolint:errcheck

	var roomsForSlots []*model.RoomsForSlot
	if err := cur.All(ctx, &roomsForSlots); err != nil {
		log.Error().Err(err).Str("collectionName", collectionRoomsForSlots).
			Msg("cannot decode rooms for slots")
		return nil, err
	}

	return roomsForSlots, nil
}

func (db *DB) RoomsForSlot(ctx context.Context, day int, time int) (*model.RoomsForSlot, error) {
	collection := db.getCollectionSemester(collectionRoomsForSlots)

	filter := bson.M{
		"$and": []bson.M{
			{"day": day},
			{"slot": time},
		},
	}

	res := collection.FindOne(ctx, filter)
	var roomsForSlot model.RoomsForSlot

	err := res.Decode(&roomsForSlot)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionRoomsForSlots).
			Int("day", day).Int("slot", time).
			Msg("Cannot decode to rooms for slot")
		return nil, err
	}

	return &roomsForSlot, nil
}

func (db *DB) SaveRoomsForSlots(ctx context.Context, roomsForSlots []*model.RoomsForSlot) error {
	collection := db.getCollectionSemester(collectionRoomsForSlots)

	err := collection.Drop(ctx)
	if err != nil {
		log.Error().Err(err).
			Str("collectionName", collectionRoomsForSlots).
			Msg("cannot drop collection")
		return err
	}
	roomsForSlotsToInsert := make([]interface{}, 0, len(roomsForSlots))
	for _, roomForSlot := range roomsForSlots {
		roomsForSlotsToInsert = append(roomsForSlotsToInsert, roomForSlot)
	}
	_, err = collection.InsertMany(ctx, roomsForSlotsToInsert)
	if err != nil {
		log.Error().Err(err).
			Str("collectionName", collectionRoomsForSlots).
			Msg("cannot insert rooms for slots")
		return err
	}
	return nil
}

func (db *DB) PrePlannedRooms(ctx context.Context) ([]*model.PrePlannedRoom, error) {
	collection := db.getCollectionSemester(collectionRoomsPrePlanned)

	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Str("collection", collectionRoomsPrePlanned).Msg("MongoDB Find")
		return nil, err
	}
	defer cur.Close(ctx) //nolint:errcheck

	rooms := make([]*model.PrePlannedRoom, 0)
	err = cur.All(ctx, &rooms)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionRoomsPrePlanned).Msg("Cannot decode to rooms")
		return nil, err
	}

	return rooms, nil
}

func (db *DB) PrePlannedRoomsForExam(ctx context.Context, ancode int) ([]*model.PrePlannedRoom, error) {
	collection := db.getCollectionSemester(collectionRoomsPrePlanned)

	cur, err := collection.Find(ctx, bson.M{"ancode": ancode})
	if err != nil {
		log.Error().Err(err).Str("collection", collectionRoomsPrePlanned).Msg("MongoDB Find")
		return nil, err
	}
	defer cur.Close(ctx) //nolint:errcheck

	rooms := make([]*model.PrePlannedRoom, 0)
	err = cur.All(ctx, &rooms)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionRoomsPrePlanned).Msg("Cannot decode to rooms")
		return nil, err
	}

	return rooms, nil
}

func (db *DB) AddPrePlannedRoomToExam(ctx context.Context, prePlannedRoom *model.PrePlannedRoom) (bool, error) {
	collection := db.getCollectionSemester(collectionRoomsPrePlanned)
	// Delete any existing document with the same ancode, room, and mtknr
	filter := bson.M{
		"ancode":   prePlannedRoom.Ancode,
		"roomname": prePlannedRoom.RoomName,
		"mtknr":    prePlannedRoom.Mtknr,
	}
	_, err := collection.DeleteOne(ctx, filter)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionRoomsPrePlanned).
			Int("ancode", prePlannedRoom.Ancode).Str("roomname", prePlannedRoom.RoomName).
			Interface("mtknr", prePlannedRoom.Mtknr).
			Msg("cannot delete existing pre planned room")
		return false, err
	}
	_, err = collection.InsertOne(ctx, prePlannedRoom)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionRoomsPrePlanned).
			Int("ancode", prePlannedRoom.Ancode).Str("roomname", prePlannedRoom.RoomName).
			Msg("cannot insert pre planned room")
		return false, err
	}
	return true, nil
}

func (db *DB) PlannedRoomNames(ctx context.Context) ([]string, error) {
	collection := db.getCollectionSemester(collectionRoomsPlanned)

	rawNames, err := collection.Distinct(ctx, "roomname", bson.D{})
	if err != nil {
		log.Error().Err(err).Msg("cannot find distinct room names")
	}

	names := make([]string, 0, len(rawNames))
	for _, rawName := range rawNames {
		name, ok := rawName.(string)
		if !ok {
			log.Debug().Interface("raw name", rawName).Msg("cannot convert to string")
		}
		names = append(names, name)
	}

	return names, nil
}

func (db *DB) PlannedRoomNamesInSlot(ctx context.Context, day, slot int) ([]string, error) {
	collection := db.getCollectionSemester(collectionRoomsPlanned)

	filter := bson.M{"day": day, "slot": slot}

	rawNames, err := collection.Distinct(ctx, "roomname", filter)
	if err != nil {
		log.Error().Err(err).Int("day", day).Int("slot", slot).Msg("cannot find roomnames for slot")
		return nil, err
	}

	names := make([]string, 0, len(rawNames))
	for _, rawName := range rawNames {
		name, ok := rawName.(string)
		if !ok {
			log.Debug().Interface("raw name", rawName).Msg("cannot convert to string")
		}
		names = append(names, name)
	}

	return names, nil
}

func (db *DB) PlannedRooms(ctx context.Context) ([]*model.PlannedRoom, error) {
	collection := db.getCollectionSemester(collectionRoomsPlanned)

	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Msg("cannot find planned rooms")
		return nil, err
	}

	plannedRooms := make([]*model.PlannedRoom, 0)
	err = cur.All(ctx, &plannedRooms)
	if err != nil {
		log.Error().Err(err).Msg("cannot decode planned rooms")
		return nil, err
	}

	return plannedRooms, nil
}

func (db *DB) PlannedRoomsInSlot(ctx context.Context, day, slot int) ([]*model.PlannedRoom, error) {
	collection := db.getCollectionSemester(collectionRoomsPlanned)

	filter := bson.M{"day": day, "slot": slot}

	cur, err := collection.Find(ctx, filter)
	if err != nil {
		log.Error().Err(err).Int("day", day).Int("slot", slot).Msg("cannot find rooms for slot")
		return nil, err
	}

	plannedRooms := make([]*model.PlannedRoom, 0)
	err = cur.All(ctx, &plannedRooms)
	if err != nil {
		log.Error().Err(err).Int("day", day).Int("slot", slot).Msg("cannot decode rooms for slot")
		return nil, err
	}

	return plannedRooms, nil
}

func (db *DB) PlannedRoomsForAncode(ctx context.Context, ancode int) ([]*model.PlannedRoom, error) {
	collection := db.getCollectionSemester(collectionRoomsPlanned)

	filter := bson.M{"ancode": ancode}

	cur, err := collection.Find(ctx, filter)
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot find rooms for ancode")
		return nil, err
	}

	plannedRooms := make([]*model.PlannedRoom, 0)
	err = cur.All(ctx, &plannedRooms)
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot decode rooms for ancode")
		return nil, err
	}

	return plannedRooms, nil
}

func (db *DB) ReplacePlannedRooms(ctx context.Context, plannedRooms []*model.PlannedRoom) error {
	collection := db.getCollectionSemester(collectionRoomsPlanned)

	err := collection.Drop(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot drop planned rooms")
		return err
	}

	roomsInterface := make([]interface{}, 0, len(plannedRooms))

	for _, room := range plannedRooms {
		roomsInterface = append(roomsInterface, room)
	}

	_, err = collection.InsertMany(ctx, roomsInterface)
	if err != nil {
		log.Error().Err(err).Msg("cannot insert non NTA rooms")
		return err
	}

	return nil
}
