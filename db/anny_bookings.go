package db

import (
	"context"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type AnnyBooking struct {
	Number               string     `bson:"number"`
	StartDate            time.Time  `bson:"start_date"`
	EndDate              time.Time  `bson:"end_date"`
	BlockerStartDate     time.Time  `bson:"blocker_start_date"`
	BlockerEndDate       time.Time  `bson:"blocker_end_date"`
	ChargedDuration      int        `bson:"charged_duration"`
	Description          string     `bson:"description"`
	CreatedAt            time.Time  `bson:"created_at"`
	UpdatedAt            time.Time  `bson:"updated_at"`
	CanceledAt           *time.Time `bson:"canceled_at,omitempty"`
	Status               string     `bson:"status"`
	StatusReason         any        `bson:"status_reason,omitempty"`
	IsBlocker            bool       `bson:"is_blocker"`
	CanEdit              bool       `bson:"can_edit"`
	IsEditable           bool       `bson:"is_editable"`
	ManuallyCreated      bool       `bson:"manually_created"`
	Note                 string     `bson:"note"`
	Room                 string     `bson:"room,omitempty"`
	Self                 string     `bson:"self"`
	PersonalizationName  string     `bson:"personalization_name"`
	BookingGroupID       string     `bson:"booking_group_identifier,omitempty"`
	CancelableUntil      *time.Time `bson:"cancelable_until,omitempty"`
	HasCustomDescription bool       `bson:"has_custom_description"`
	ResourceID           string     `bson:"resource_id,omitempty"`
}

func (db *DB) SaveAnnyBookings(ctx context.Context, bookings []*AnnyBooking) error {
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

func (db *DB) AnnyBookings(ctx context.Context, room *string) ([]*AnnyBooking, error) {
	return db.annyBookings(ctx, room)
}

func (db *DB) AllAnnyBookings(ctx context.Context) ([]*AnnyBooking, error) {
	return db.annyBookings(ctx, nil)
}

func (db *DB) annyBookings(ctx context.Context, room *string) ([]*AnnyBooking, error) {
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

	bookings := make([]*AnnyBooking, 0)
	if err := cur.All(ctx, &bookings); err != nil {
		return nil, err
	}

	return bookings, nil
}

func (db *DB) AnnyBookingsCollection(ctx context.Context) *mongo.Collection {
	return db.Client.Database(db.databaseName).Collection(collectionAnnyBookings)
}
