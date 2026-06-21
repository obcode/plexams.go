package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// BlockedRooms returns all room-slot blocks of the semester, sorted by
// room/day/slot.
func (db *DB) BlockedRooms(ctx context.Context) ([]*model.BlockedRoom, error) {
	collection := db.getCollectionSemester(collectionRoomsBlocked)
	cur, err := collection.Find(ctx, bson.M{},
		options.Find().SetSort(bson.D{{Key: "room", Value: 1}, {Key: "day", Value: 1}, {Key: "slot", Value: 1}}))
	if err != nil {
		log.Error().Err(err).Msg("cannot get blocked rooms")
		return nil, err
	}
	blocked := make([]*model.BlockedRoom, 0)
	if err := cur.All(ctx, &blocked); err != nil {
		log.Error().Err(err).Msg("cannot decode blocked rooms")
		return nil, err
	}
	return blocked, nil
}

// BlockRoomForSlot stores (or updates) a room-slot block (key: room/day/slot).
func (db *DB) BlockRoomForSlot(ctx context.Context, block *model.BlockedRoom) error {
	collection := db.getCollectionSemester(collectionRoomsBlocked)
	filter := bson.M{"room": block.Room, "day": block.Day, "slot": block.Slot}
	if _, err := collection.ReplaceOne(ctx, filter, block, options.Replace().SetUpsert(true)); err != nil {
		log.Error().Err(err).Str("room", block.Room).Msg("cannot block room for slot")
		return err
	}
	return nil
}

// UnblockRoomForSlot removes a room-slot block (key: room/day/slot). It reports
// whether a block was actually removed.
func (db *DB) UnblockRoomForSlot(ctx context.Context, room string, day, slot int) (bool, error) {
	collection := db.getCollectionSemester(collectionRoomsBlocked)
	res, err := collection.DeleteOne(ctx, bson.M{"room": room, "day": day, "slot": slot})
	if err != nil {
		log.Error().Err(err).Str("room", room).Msg("cannot unblock room for slot")
		return false, err
	}
	return res.DeletedCount > 0, nil
}
