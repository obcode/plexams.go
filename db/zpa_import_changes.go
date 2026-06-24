package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// SaveZPAImportChange stores the change record of the most recent import of one
// kind (upsert by kind, so only the latest import per kind is kept).
func (db *DB) SaveZPAImportChange(ctx context.Context, change *model.ZPAImportChange) error {
	collection := db.getCollectionSemester(collectionZPAImportChanges)
	_, err := collection.ReplaceOne(ctx,
		bson.M{"kind": change.Kind},
		change,
		options.Replace().SetUpsert(true))
	if err != nil {
		log.Error().Err(err).Str("kind", change.Kind).Msg("cannot save zpa import change")
		return err
	}
	return nil
}

// ZPAImportChanges returns the stored change records (latest import per kind).
func (db *DB) ZPAImportChanges(ctx context.Context) ([]*model.ZPAImportChange, error) {
	collection := db.getCollectionSemester(collectionZPAImportChanges)
	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Msg("cannot find zpa import changes")
		return nil, err
	}
	changes := make([]*model.ZPAImportChange, 0)
	if err := cur.All(ctx, &changes); err != nil {
		log.Error().Err(err).Msg("cannot decode zpa import changes")
		return nil, err
	}
	return changes, nil
}
