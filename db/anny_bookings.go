package db

import (
	"context"
	"strings"

	"github.com/obcode/plexams.go/graph/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (db *DB) SaveAnnyBookings(ctx context.Context, bookings []*model.AnnyBooking) error {
	collection := db.Client.Database(db.databaseName).Collection(collectionAnnyBookings)

	err := collection.Drop(ctx)
	if err != nil {
		return err
	}

	entries := make([]interface{}, 0, len(bookings))
	for _, booking := range bookings {
		entries = append(entries, booking)
	}

	_, err = collection.InsertMany(ctx, entries)
	if err != nil {
		return err
	}

	return nil
}

func (db *DB) AnnyBookings(ctx context.Context, room *string) ([]*model.AnnyBooking, error) {
	return db.annyBookings(ctx, room)
}

func (db *DB) AllAnnyBookings(ctx context.Context) ([]*model.AnnyBooking, error) {
	return db.annyBookings(ctx, nil)
}

func (db *DB) annyBookings(ctx context.Context, room *string) ([]*model.AnnyBooking, error) {
	collection := db.Client.Database(db.databaseName).Collection(collectionAnnyBookings)

	filter := bson.M{}
	if room != nil && strings.TrimSpace(*room) != "" {
		filter["room"] = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(*room), " ", ""))
	}

	findOptions := options.Find()
	findOptions.SetSort(bson.D{{Key: "start_date", Value: 1}})

	cur, err := collection.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx) //nolint:errcheck

	bookings := make([]*model.AnnyBooking, 0)
	if err := cur.All(ctx, &bookings); err != nil {
		return nil, err
	}

	return bookings, nil
}

func (db *DB) AnnyBookingsCollection(ctx context.Context) *mongo.Collection {
	return db.Client.Database(db.databaseName).Collection(collectionAnnyBookings)
}
