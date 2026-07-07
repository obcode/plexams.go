package db

import (
	"context"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// BlockedRooms returns all room blocks of the semester, sorted by room/starttime.
func (db *DB) BlockedRooms(ctx context.Context) ([]*model.BlockedRoom, error) {
	collection := db.getCollectionSemester(collectionRoomsBlocked)
	cur, err := collection.Find(ctx, bson.M{},
		options.Find().SetSort(bson.D{{Key: "room", Value: 1}, {Key: "starttime", Value: 1}}))
	if err != nil {
		log.Error().Err(err).Msg("cannot get blocked rooms")
		return nil, err
	}
	blocked := make([]*model.BlockedRoom, 0)
	if err := cur.All(ctx, &blocked); err != nil {
		log.Error().Err(err).Msg("cannot decode blocked rooms")
		return nil, err
	}
	for _, br := range blocked {
		db.decorateBlockedRoom(br)
	}
	return blocked, nil
}

// BlockRoomForSlot stores (or updates) a room block (key: room + starttime). The
// block's Starttime must be set by the caller (from the slot's start time).
func (db *DB) BlockRoomForSlot(ctx context.Context, block *model.BlockedRoom) error {
	collection := db.getCollectionSemester(collectionRoomsBlocked)
	filter := bson.M{"room": block.Room, "starttime": block.Starttime}
	if _, err := collection.ReplaceOne(ctx, filter, block, options.Replace().SetUpsert(true)); err != nil {
		log.Error().Err(err).Str("room", block.Room).Msg("cannot block room for slot")
		return err
	}
	return nil
}

// UnblockRoomForSlot removes a room block (key: room + starttime). It reports
// whether a block was actually removed.
func (db *DB) UnblockRoomForSlot(ctx context.Context, room string, starttime time.Time) (bool, error) {
	collection := db.getCollectionSemester(collectionRoomsBlocked)
	res, err := collection.DeleteOne(ctx, bson.M{"room": room, "starttime": starttime})
	if err != nil {
		log.Error().Err(err).Str("room", room).Msg("cannot unblock room for slot")
		return false, err
	}
	return res.DeletedCount > 0, nil
}
