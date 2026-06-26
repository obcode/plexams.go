package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// UpsertSpecialInterest creates or updates one special-interest group (key: name).
func (db *DB) UpsertSpecialInterest(ctx context.Context, si *model.SpecialInterest) error {
	collection := db.getCollectionSemester(collectionSpecialInterests)
	_, err := collection.ReplaceOne(ctx, bson.M{"name": si.Name}, si, options.Replace().SetUpsert(true))
	if err != nil {
		log.Error().Err(err).Str("name", si.Name).Msg("cannot upsert special interest")
		return err
	}
	return nil
}

// DeleteSpecialInterest removes one special-interest group by name.
func (db *DB) DeleteSpecialInterest(ctx context.Context, name string) (bool, error) {
	collection := db.getCollectionSemester(collectionSpecialInterests)
	res, err := collection.DeleteOne(ctx, bson.M{"name": name})
	if err != nil {
		log.Error().Err(err).Str("name", name).Msg("cannot delete special interest")
		return false, err
	}
	return res.DeletedCount > 0, nil
}

// SpecialInterests returns all special-interest groups.
func (db *DB) SpecialInterests(ctx context.Context) ([]*model.SpecialInterest, error) {
	collection := db.getCollectionSemester(collectionSpecialInterests)
	cur, err := collection.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		log.Error().Err(err).Msg("cannot find special interests")
		return nil, err
	}
	sis := make([]*model.SpecialInterest, 0)
	if err := cur.All(ctx, &sis); err != nil {
		log.Error().Err(err).Msg("cannot decode special interests")
		return nil, err
	}
	return sis, nil
}
