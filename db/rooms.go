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

// HasRoom reports whether a room with the given name exists.
func (db *DB) HasRoom(ctx context.Context, name string) (bool, error) {
	collection := db.Client.Database("plexams").Collection(collectionGlobalRooms)
	count, err := collection.CountDocuments(ctx, bson.M{"name": name})
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// AddRoom inserts a new room and returns it.
func (db *DB) AddRoom(ctx context.Context, room *model.Room) (*model.Room, error) {
	collection := db.Client.Database("plexams").Collection(collectionGlobalRooms)
	if _, err := collection.InsertOne(ctx, room); err != nil {
		log.Error().Err(err).Str("room", room.Name).Msg("cannot insert room")
		return nil, err
	}
	return db.RoomByName(ctx, room.Name)
}

// ReplaceRoom replaces the room identified by its name (no upsert) and returns
// it. Errors if no room with that name exists.
func (db *DB) ReplaceRoom(ctx context.Context, room *model.Room) (*model.Room, error) {
	collection := db.Client.Database("plexams").Collection(collectionGlobalRooms)
	res, err := collection.ReplaceOne(ctx, bson.M{"name": room.Name}, room)
	if err != nil {
		log.Error().Err(err).Str("room", room.Name).Msg("cannot replace room")
		return nil, err
	}
	if res.MatchedCount == 0 {
		return nil, fmt.Errorf("cannot find room %s", room.Name)
	}
	return db.RoomByName(ctx, room.Name)
}

// SetRoomRequestWith sets the requestWith and (derived) needsRequest fields of
// the room identified by name.
func (db *DB) SetRoomRequestWith(ctx context.Context, name, requestWith string, needsRequest bool) error {
	collection := db.Client.Database("plexams").Collection(collectionGlobalRooms)
	_, err := collection.UpdateOne(ctx,
		bson.M{"name": name},
		bson.D{{Key: "$set", Value: bson.D{
			{Key: "requestwith", Value: requestWith},
			{Key: "needsrequest", Value: needsRequest},
		}}})
	if err != nil {
		log.Error().Err(err).Str("room", name).Msg("cannot set room requestWith")
	}
	return err
}

// SetRoomDeactivated sets the deactivated flag of the room identified by name.
// Returns an error if no room with that name exists.
func (db *DB) SetRoomDeactivated(ctx context.Context, name string, deactivated bool) (*model.Room, error) {
	collection := db.Client.Database("plexams").Collection(collectionGlobalRooms)

	res, err := collection.UpdateOne(ctx,
		bson.M{"name": name},
		bson.D{{Key: "$set", Value: bson.D{{Key: "deactivated", Value: deactivated}}}})
	if err != nil {
		log.Error().Err(err).Str("room", name).Msg("cannot set room deactivated flag")
		return nil, err
	}
	if res.MatchedCount == 0 {
		return nil, fmt.Errorf("cannot find room %s", name)
	}
	return db.RoomByName(ctx, name)
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

	findOptions := options.Find()
	findOptions.SetSort(bson.D{{Key: "roomname", Value: 1}})

	cur, err := collection.Find(ctx, bson.M{"ancode": ancode}, findOptions)
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

// RemovePrePlannedRoomFromExam deletes a pre-planned room from an exam (key:
// ancode/roomName/mtknr). It reports whether a document was actually removed.
func (db *DB) RemovePrePlannedRoomFromExam(ctx context.Context, ancode int, roomName string, mtknr *string) (bool, error) {
	collection := db.getCollectionSemester(collectionRoomsPrePlanned)
	filter := bson.M{
		"ancode":   ancode,
		"roomname": roomName,
		"mtknr":    mtknr,
	}
	res, err := collection.DeleteOne(ctx, filter)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionRoomsPrePlanned).
			Int("ancode", ancode).Str("roomname", roomName).Interface("mtknr", mtknr).
			Msg("cannot delete pre planned room")
		return false, err
	}
	return res.DeletedCount > 0, nil
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

	findOptions := options.Find()
	findOptions.SetSort(bson.D{{Key: "roomname", Value: 1}})

	cur, err := collection.Find(ctx, filter, findOptions)
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

// ResetPlannedRooms drops the generated planned rooms (planned_rooms). The
// pre-planning (rooms_pre_planned) is not touched.
func (db *DB) ResetPlannedRooms(ctx context.Context) error {
	collection := db.getCollectionSemester(collectionRoomsPlanned)
	if err := collection.Drop(ctx); err != nil {
		log.Error().Err(err).Msg("cannot drop planned rooms")
		return err
	}
	return nil
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
