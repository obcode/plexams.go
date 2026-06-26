package db

import (
	"context"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// AddMutationLogEntry appends one mutating-operation entry to the per-semester
// mutation_log collection.
func (db *DB) AddMutationLogEntry(ctx context.Context, entry *model.MutationLogEntry) error {
	collection := db.getCollectionSemester(collectionMutationLog)
	if _, err := collection.InsertOne(ctx, entry); err != nil {
		log.Error().Err(err).Str("name", entry.Name).Msg("cannot add mutation-log entry")
		return err
	}
	return nil
}

// MutationLog returns the mutation log, newest first, filtered by operation name,
// a referenced ancode, argument key/value pairs and/or a time range. limit <= 0
// returns all.
func (db *DB) MutationLog(ctx context.Context, opType, name *string, ancode *int,
	argFilters []*model.ArgFilterInput, since, until *time.Time, limit int) ([]*model.MutationLogEntry, error) {
	collection := db.getCollectionSemester(collectionMutationLog)

	filter := bson.M{}
	if opType != nil && *opType != "" {
		filter["type"] = *opType
	}
	if name != nil && *name != "" {
		filter["name"] = *name
	}
	if ancode != nil {
		filter["ancodes"] = *ancode
	}
	// each argument filter matches one element of the args array
	and := make([]bson.M, 0, len(argFilters))
	for _, af := range argFilters {
		if af == nil {
			continue
		}
		and = append(and, bson.M{"args": bson.M{"$elemMatch": bson.M{"key": af.Key, "value": af.Value}}})
	}
	if len(and) > 0 {
		filter["$and"] = and
	}
	if since != nil || until != nil {
		timeFilter := bson.M{}
		if since != nil {
			timeFilter["$gte"] = *since
		}
		if until != nil {
			timeFilter["$lte"] = *until
		}
		filter["time"] = timeFilter
	}

	opts := options.Find().SetSort(bson.D{{Key: "time", Value: -1}})
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}

	cur, err := collection.Find(ctx, filter, opts)
	if err != nil {
		log.Error().Err(err).Msg("cannot find mutation log")
		return nil, err
	}
	entries := make([]*model.MutationLogEntry, 0)
	if err := cur.All(ctx, &entries); err != nil {
		log.Error().Err(err).Msg("cannot decode mutation log")
		return nil, err
	}
	return entries, nil
}

// MutationLogNames returns the distinct operation names present in the log.
func (db *DB) MutationLogNames(ctx context.Context) ([]string, error) {
	collection := db.getCollectionSemester(collectionMutationLog)
	values, err := collection.Distinct(ctx, "name", bson.M{})
	if err != nil {
		log.Error().Err(err).Msg("cannot get distinct mutation-log names")
		return nil, err
	}
	names := make([]string, 0, len(values))
	for _, v := range values {
		if s, ok := v.(string); ok {
			names = append(names, s)
		}
	}
	return names, nil
}
