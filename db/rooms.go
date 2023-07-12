package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (db *DB) GlobalRooms(ctx context.Context) ([]*model.Room, error) {
	collection := db.Client.Database("plexams").Collection(collectionRooms)

	findOptions := options.Find()
	findOptions.SetSort(bson.D{{Key: "name", Value: 1}})

	cur, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionRooms).Msg("MongoDB Find")
		return nil, err
	}
	defer cur.Close(ctx)

	rooms := make([]*model.Room, 0)
	err = cur.All(ctx, &rooms)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionRooms).Msg("Cannot decode to rooms")
		return nil, err
	}

	return rooms, nil
}

func (db *DB) Rooms(ctx context.Context) ([]*model.Room, error) {

	return nil, nil
}

func (db *DB) SaveRooms(ctx context.Context, slotsWithRooms []*model.SlotWithRooms) error {
	collection := db.Client.Database(db.databaseName).Collection(collectionRooms)

	err := collection.Drop(ctx)
	if err != nil {
		log.Error().Err(err).
			Str("collectionName", collectionRooms).
			Msg("cannot drop collection")
		return err
	}

	slotsWithRoomsToInsert := make([]interface{}, 0, len(slotsWithRooms))
	for _, slotWithRooms := range slotsWithRooms {
		slotsWithRoomsToInsert = append(slotsWithRoomsToInsert, slotWithRooms)
	}

	_, err = collection.InsertMany(ctx, slotsWithRoomsToInsert)
	if err != nil {
		log.Error().Err(err).
			Str("collectionName", collectionNameNTAs).
			Msg("cannot insert rooms")
		return err
	}

	return nil
}

func (db *DB) RoomsForSlot(ctx context.Context, day int, time int) (*model.SlotWithRooms, error) {
	collection := db.getCollectionSemester(collectionRooms)

	filter := bson.M{
		"$and": []bson.M{
			{"daynumber": day},
			{"slotnumber": time},
		},
	}

	res := collection.FindOne(ctx, filter)
	var slotWithRooms model.SlotWithRooms

	err := res.Decode(&slotWithRooms)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionRooms).
			Int("daynumber", day).Int("slotnumber", time).
			Msg("Cannot decode to slot with rooms")
		return nil, err
	}

	return &slotWithRooms, nil
}

func (db *DB) RoomPlannedInSlot(ctx context.Context, roomName string, day int, time int) ([]*model.RoomForExam, error) {
	collection := db.getCollectionSemester(collectionRooms)

	filter := bson.M{
		"$and": []bson.M{
			{"room.roomName": roomName},
			{"daynumber": day},
			{"slotnumber": time},
		},
	}

	cur, err := collection.Find(ctx, filter)
	if err != nil {
		log.Error().Err(err).Msg("error while trying to find rooms planned in slot")
		return nil, err
	}

	roomsForExam := make([]*model.RoomForExam, 0)

	err = cur.All(ctx, &roomsForExam)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionRooms).Msg("Cannot decode to rooms for exams")
		return nil, err
	}

	return roomsForExam, nil
}

func (db *DB) AddRoomToExam(ctx context.Context, room *model.RoomForExam) error {
	collection := db.getCollectionSemester(collectionRoomsForExams)

	_, err := collection.InsertOne(ctx, room)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionRoomsForExams).Msg("cannot insert room into collection")
		return err
	}

	return nil
}

func (db *DB) RoomsForAncode(ctx context.Context, ancode int) ([]*model.RoomForExam, error) {
	collection := db.getCollectionSemester(collectionRoomsForExams)

	cur, err := collection.Find(ctx, bson.D{{Key: "ancode", Value: ancode}})
	if err != nil {
		log.Error().Err(err).Str("collection", collectionRooms).Int("ancode", ancode).
			Msg("error while trying to find rooms for ancode")
		return nil, err
	}

	roomsForExam := make([]*model.RoomForExam, 0)

	err = cur.All(ctx, &roomsForExam)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionRooms).Int("ancode", ancode).
			Msg("Cannot decode to rooms for exams")
		return nil, err
	}

	return roomsForExam, nil
}

func (db *DB) RoomsPlannedInSlot(ctx context.Context, day, time int) ([]*model.RoomForExam, error) {
	exams, err := db.ExamsInSlot(ctx, day, time)
	if err != nil {
		log.Error().Err(err).Int("day", day).Int("time", time).Msg("error while getting exams in slot")
		return nil, err
	}

	rooms := make([]*model.RoomForExam, 0)
	for _, exam := range exams {
		roomsForAncode, err := db.RoomsForAncode(ctx, exam.Exam.Ancode)
		if err != nil {
			log.Error().Err(err).Int("ancode", exam.Exam.Ancode).Msg("error while getting rooms for ancode")
			return nil, err
		}

		rooms = append(rooms, roomsForAncode...)
	}

	return rooms, nil
}

func (db *DB) ChangeRoom(ctx context.Context, ancode int, oldRoom, newRoom *model.Room) (bool, error) {
	// TODO: Implement db.ChangeRoom
	return false, nil
}

func (db *DB) PlannedRoomNames(ctx context.Context) ([]string, error) {
	collection := db.getCollectionSemester(collectionRoomsForExams)

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
