package db

import (
	"context"
	"sort"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
)

const collectionConstraints = "constraints"

func (db *DB) NotPlannedByMe(ctx context.Context, ancode int) (bool, error) {
	constraint, err := db.GetConstraintsForAncode(ctx, ancode)
	if err != nil {
		return false, err
	}

	update := false
	if constraint == nil {
		constraint = &model.Constraints{Ancode: ancode}
	} else {
		update = true
	}

	constraint.NotPlannedByMe = true

	collection := db.Client.Database(databaseName(db.semester)).Collection(collectionConstraints)
	if update {
		_, err = collection.ReplaceOne(ctx, bson.D{{Key: "ancode", Value: ancode}}, constraint)
	} else {
		_, err = collection.InsertOne(ctx, constraint)
	}
	if err != nil {
		return false, err
	}

	return true, nil
}

func (db *DB) Online(ctx context.Context, ancode int) (bool, error) {
	constraint, err := db.GetConstraintsForAncode(ctx, ancode)
	if err != nil {
		return false, err
	}

	update := false
	if constraint == nil {
		constraint = &model.Constraints{Ancode: ancode}
	} else {
		update = true
	}

	constraint.Online = true

	collection := db.Client.Database(databaseName(db.semester)).Collection(collectionConstraints)
	if update {
		_, err = collection.ReplaceOne(ctx, bson.D{{Key: "ancode", Value: ancode}}, constraint)
	} else {
		_, err = collection.InsertOne(ctx, constraint)
	}
	if err != nil {
		return false, err
	}

	return true, nil
}

func (db *DB) ExcludeDays(ctx context.Context, ancode int, days []*time.Time) (bool, error) {
	constraint, err := db.GetConstraintsForAncode(ctx, ancode)
	if err != nil {
		return false, err
	}

	update := false
	if constraint == nil {
		constraint = &model.Constraints{Ancode: ancode}
	} else {
		update = true
	}

	constraint.ExcludeDays = days

	collection := db.Client.Database(databaseName(db.semester)).Collection(collectionConstraints)

	if update {
		_, err = collection.ReplaceOne(ctx, bson.D{{Key: "ancode", Value: ancode}}, constraint)
	} else {
		_, err = collection.InsertOne(ctx, constraint)
	}
	if err != nil {
		return false, err
	}

	return true, nil
}

func (db *DB) SameSlot(ctx context.Context, ancode int, ancodes []int) (bool, error) {
	constraint, err := db.GetConstraintsForAncode(ctx, ancode)
	if err != nil {
		return false, err
	}
	update := false
	if constraint == nil {
		constraint = &model.Constraints{Ancode: ancode}
	} else {
		update = true
	}

	if constraint.SameSlot == nil {
		constraint.SameSlot = ancodes
	} else {
		constraint.SameSlot = append(constraint.SameSlot, ancodes...)
	}

	// remove duplicates
	allKeys := make(map[int]bool)
	list := []int{}
	for _, item := range constraint.SameSlot {
		if _, value := allKeys[item]; !value {
			allKeys[item] = true
			list = append(list, item)
		}
	}

	// and sort
	sort.Ints(list)
	constraint.SameSlot = list

	collection := db.Client.Database(databaseName(db.semester)).Collection(collectionConstraints)
	if update {
		_, err = collection.ReplaceOne(ctx, bson.D{{Key: "ancode", Value: ancode}}, constraint)
	} else {
		_, err = collection.InsertOne(ctx, constraint)
	}
	if err != nil {
		return false, err
	}

	return true, nil
}

func (db *DB) PlacesWithSockets(ctx context.Context, ancode int) (bool, error) {
	constraint, err := db.GetConstraintsForAncode(ctx, ancode)
	if err != nil {
		return false, err
	}
	update := false
	if constraint == nil {
		constraint = &model.Constraints{Ancode: ancode}
	} else {
		update = true
	}

	if constraint.RoomConstraints == nil {
		constraint.RoomConstraints = &model.RoomConstraints{}
	}

	constraint.RoomConstraints.PlacesWithSocket = true

	collection := db.Client.Database(databaseName(db.semester)).Collection(collectionConstraints)
	if update {
		_, err = collection.ReplaceOne(ctx, bson.D{{Key: "ancode", Value: ancode}}, constraint)
	} else {
		_, err = collection.InsertOne(ctx, constraint)
	}
	if err != nil {
		return false, err
	}

	return true, nil
}

func (db *DB) ExahmRooms(ctx context.Context, ancode int) (bool, error) {
	constraint, err := db.GetConstraintsForAncode(ctx, ancode)
	if err != nil {
		return false, err
	}
	update := false
	if constraint == nil {
		constraint = &model.Constraints{Ancode: ancode}
	} else {
		update = true
	}

	if constraint.RoomConstraints == nil {
		constraint.RoomConstraints = &model.RoomConstraints{}
	}

	constraint.RoomConstraints.ExahmRooms = true

	collection := db.Client.Database(databaseName(db.semester)).Collection(collectionConstraints)
	if update {
		_, err = collection.ReplaceOne(ctx, bson.D{{Key: "ancode", Value: ancode}}, constraint)
	} else {
		_, err = collection.InsertOne(ctx, constraint)
	}
	if err != nil {
		return false, err
	}

	return true, nil
}

func (db *DB) GetConstraintsForAncode(ctx context.Context, ancode int) (*model.Constraints, error) {
	collection := db.Client.Database(databaseName(db.semester)).Collection(collectionConstraints)

	var constraint model.Constraints
	res := collection.FindOne(ctx, bson.D{{Key: "ancode", Value: ancode}})
	if res.Err() != nil {
		log.Error().Err(res.Err()).Int("ancode", ancode).Msg("no constraint found")
		return nil, nil // no constraint available
	}
	err := res.Decode(&constraint)
	if err != nil {
		log.Error().Err(res.Err()).Int("ancode", ancode).Msg("cannot decode constraint")
		return nil, err
	}

	return &constraint, nil
}
