package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// AddSyncLogEntry appends one transfer event to the sync-log history.
func (db *DB) AddSyncLogEntry(ctx context.Context, entry *model.SyncLogEntry) error {
	collection := db.getCollectionSemester(collectionSyncLog)
	if _, err := collection.InsertOne(ctx, entry); err != nil {
		log.Error().Err(err).Str("operation", entry.Operation).Msg("cannot add sync-log entry")
		return err
	}
	return nil
}

// SyncLog returns the transfer history, newest first. limit <= 0 returns all.
func (db *DB) SyncLog(ctx context.Context, limit int) ([]*model.SyncLogEntry, error) {
	collection := db.getCollectionSemester(collectionSyncLog)
	opts := options.Find().SetSort(bson.D{{Key: "time", Value: -1}})
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}
	cur, err := collection.Find(ctx, bson.M{}, opts)
	if err != nil {
		log.Error().Err(err).Msg("cannot find sync-log")
		return nil, err
	}
	entries := make([]*model.SyncLogEntry, 0)
	if err := cur.All(ctx, &entries); err != nil {
		log.Error().Err(err).Msg("cannot decode sync-log")
		return nil, err
	}
	return entries, nil
}
