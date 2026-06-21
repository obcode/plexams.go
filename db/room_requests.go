package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// RoomRequests returns all building-management room requests of the semester,
// sorted by room, day, slot.
func (db *DB) RoomRequests(ctx context.Context) ([]*model.RoomRequest, error) {
	collection := db.getCollectionSemester(collectionRoomRequests)
	cur, err := collection.Find(ctx, bson.M{},
		options.Find().SetSort(bson.D{{Key: "room", Value: 1}, {Key: "day", Value: 1}, {Key: "slot", Value: 1}}))
	if err != nil {
		log.Error().Err(err).Msg("cannot get room requests")
		return nil, err
	}
	requests := make([]*model.RoomRequest, 0)
	if err := cur.All(ctx, &requests); err != nil {
		log.Error().Err(err).Msg("cannot decode room requests")
		return nil, err
	}
	return requests, nil
}

// ReplaceAllRoomRequests drops the collection and inserts the given requests.
func (db *DB) ReplaceAllRoomRequests(ctx context.Context, requests []*model.RoomRequest) error {
	collection := db.getCollectionSemester(collectionRoomRequests)
	if err := collection.Drop(ctx); err != nil {
		log.Error().Err(err).Msg("cannot drop room requests collection")
		return err
	}
	if len(requests) == 0 {
		return nil
	}
	docs := make([]interface{}, 0, len(requests))
	for _, r := range requests {
		docs = append(docs, r)
	}
	if _, err := collection.InsertMany(ctx, docs); err != nil {
		log.Error().Err(err).Msg("cannot insert room requests")
		return err
	}
	return nil
}

// GetRoomRequest returns one room request (key: room/day/slot) or nil.
func (db *DB) GetRoomRequest(ctx context.Context, room string, day, slot int) (*model.RoomRequest, error) {
	collection := db.getCollectionSemester(collectionRoomRequests)
	res := collection.FindOne(ctx, bson.M{"room": room, "day": day, "slot": slot})
	if res.Err() == mongo.ErrNoDocuments {
		return nil, nil
	}
	var request model.RoomRequest
	if err := res.Decode(&request); err != nil {
		log.Error().Err(err).Str("room", room).Msg("cannot decode room request")
		return nil, err
	}
	return &request, nil
}

func (db *DB) setRoomRequestField(ctx context.Context, room string, day, slot int, field string, value bool) (*model.RoomRequest, error) {
	collection := db.getCollectionSemester(collectionRoomRequests)
	res, err := collection.UpdateOne(ctx,
		bson.M{"room": room, "day": day, "slot": slot},
		bson.D{{Key: "$set", Value: bson.D{{Key: field, Value: value}}}})
	if err != nil {
		log.Error().Err(err).Str("room", room).Str("field", field).Msg("cannot update room request")
		return nil, err
	}
	if res.MatchedCount == 0 {
		return nil, nil
	}
	return db.GetRoomRequest(ctx, room, day, slot)
}

// SetRoomRequestApproved sets the approved flag (key: room/day/slot). Returns nil
// if no such request exists.
func (db *DB) SetRoomRequestApproved(ctx context.Context, room string, day, slot int, approved bool) (*model.RoomRequest, error) {
	return db.setRoomRequestField(ctx, room, day, slot, "approved", approved)
}

// SetRoomRequestActive sets the active flag (key: room/day/slot). Returns nil if
// no such request exists.
func (db *DB) SetRoomRequestActive(ctx context.Context, room string, day, slot int, active bool) (*model.RoomRequest, error) {
	return db.setRoomRequestField(ctx, room, day, slot, "active", active)
}
