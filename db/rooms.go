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
	collection := db.Client.Database(databaseName(db.semester)).Collection(collectionRooms)

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
