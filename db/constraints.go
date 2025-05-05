package db

import (
	"context"
	"sort"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

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

	collection := db.Client.Database(db.databaseName).Collection(collectionConstraints)
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

	collection := db.Client.Database(db.databaseName).Collection(collectionConstraints)
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

	collection := db.Client.Database(db.databaseName).Collection(collectionConstraints)

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

func (db *DB) PossibleDays(ctx context.Context, ancode int, days []*time.Time) (bool, error) {
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

	constraint.PossibleDays = days

	collection := db.Client.Database(db.databaseName).Collection(collectionConstraints)

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

	collection := db.Client.Database(db.databaseName).Collection(collectionConstraints)
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

	collection := db.Client.Database(db.databaseName).Collection(collectionConstraints)
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

func (db *DB) Lab(ctx context.Context, ancode int) (bool, error) {
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

	constraint.RoomConstraints.Lab = true

	collection := db.Client.Database(db.databaseName).Collection(collectionConstraints)
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

func (db *DB) Exahm(ctx context.Context, ancode int) (bool, error) {
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

	constraint.RoomConstraints.Exahm = true

	collection := db.Client.Database(db.databaseName).Collection(collectionConstraints)
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

func (db *DB) SafeExamBrowser(ctx context.Context, ancode int) (bool, error) {
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

	constraint.RoomConstraints.Seb = true

	collection := db.Client.Database(db.databaseName).Collection(collectionConstraints)
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
	collection := db.Client.Database(db.databaseName).Collection(collectionConstraints)

	var constraint model.Constraints
	res := collection.FindOne(ctx, bson.D{{Key: "ancode", Value: ancode}})
	if res.Err() != nil {
		log.Debug().Err(res.Err()).Int("ancode", ancode).Msg("no constraint found")
		return nil, nil // no constraint available
	}
	err := res.Decode(&constraint)
	if err != nil {
		log.Error().Err(res.Err()).Int("ancode", ancode).Msg("cannot decode constraint")
		return nil, err
	}

	return &constraint, nil
}

func (db *DB) GetConstraints(ctx context.Context) ([]*model.Constraints, error) {
	collection := db.Client.Database(db.databaseName).Collection(collectionConstraints)

	constraints := make([]*model.Constraints, 0)

	findOptions := options.Find()
	findOptions.SetSort(bson.D{{Key: "ancode", Value: 1}})

	cur, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("collection", collectionConstraints).Msg("MongoDB Find")
		return constraints, err
	}
	defer cur.Close(ctx)

	// TODO: replace all cur.Next with cur.All
	for cur.Next(ctx) {
		var constraint model.Constraints

		err := cur.Decode(&constraint)
		if err != nil {
			log.Error().Err(err).Str("semester", db.semester).Str("collection", collectionConstraints).Interface("cur", cur).
				Msg("Cannot decode to additional exam")
			return constraints, err
		}

		constraints = append(constraints, &constraint)
	}

	if err := cur.Err(); err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("collection", collectionConstraints).Msg("Cursor returned error")
		return constraints, err
	}

	return constraints, nil
}

func (db *DB) AddConstraints(ctx context.Context, ancode int, constraints *model.Constraints) (*model.Constraints, error) {
	collection := db.Client.Database(db.databaseName).Collection(collectionConstraints)

	res, err := collection.ReplaceOne(ctx, bson.D{{Key: "ancode", Value: ancode}}, constraints)
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot add constraints")
		return nil, err
	}
	if res.MatchedCount == 0 {
		_, err = collection.InsertOne(ctx, constraints)
		if err != nil {
			log.Error().Err(err).Int("ancode", ancode).Msg("cannot add constraints")
			return nil, err
		}
	}
	return db.GetConstraintsForAncode(ctx, ancode)
}

func (db *DB) RmConstraints(ctx context.Context, ancode int) (bool, error) {
	collection := db.Client.Database(db.databaseName).Collection(collectionConstraints)

	_, err := collection.DeleteOne(ctx, bson.D{{Key: "ancode", Value: ancode}})
	if err != nil {
		log.Debug().Err(err).Int("ancode", ancode).Msg("cannot delete constraints")
		return false, err
	}

	return true, nil
}
