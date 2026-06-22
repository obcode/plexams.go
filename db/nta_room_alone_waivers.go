package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// NtaRoomAloneWaivers returns all accepted NTA room-alone waivers of the
// semester, sorted by mtknr/ancode.
func (db *DB) NtaRoomAloneWaivers(ctx context.Context) ([]*model.NtaRoomAloneWaiver, error) {
	collection := db.getCollectionSemester(collectionNtaRoomAloneWaivers)
	cur, err := collection.Find(ctx, bson.M{},
		options.Find().SetSort(bson.D{{Key: "mtknr", Value: 1}, {Key: "ancode", Value: 1}}))
	if err != nil {
		log.Error().Err(err).Msg("cannot get nta room-alone waivers")
		return nil, err
	}
	waivers := make([]*model.NtaRoomAloneWaiver, 0)
	if err := cur.All(ctx, &waivers); err != nil {
		log.Error().Err(err).Msg("cannot decode nta room-alone waivers")
		return nil, err
	}
	return waivers, nil
}

// AddNtaRoomAloneWaiver stores (or replaces) a waiver (key: mtknr/ancode).
func (db *DB) AddNtaRoomAloneWaiver(ctx context.Context, waiver *model.NtaRoomAloneWaiver) error {
	collection := db.getCollectionSemester(collectionNtaRoomAloneWaivers)
	filter := bson.M{"mtknr": waiver.Mtknr, "ancode": waiver.Ancode}
	if _, err := collection.ReplaceOne(ctx, filter, waiver, options.Replace().SetUpsert(true)); err != nil {
		log.Error().Err(err).Str("mtknr", waiver.Mtknr).Int("ancode", waiver.Ancode).Msg("cannot add nta room-alone waiver")
		return err
	}
	return nil
}

// RemoveNtaRoomAloneWaiver deletes a waiver (key: mtknr/ancode). It reports
// whether a document was removed.
func (db *DB) RemoveNtaRoomAloneWaiver(ctx context.Context, mtknr string, ancode int) (bool, error) {
	collection := db.getCollectionSemester(collectionNtaRoomAloneWaivers)
	res, err := collection.DeleteOne(ctx, bson.M{"mtknr": mtknr, "ancode": ancode})
	if err != nil {
		log.Error().Err(err).Str("mtknr", mtknr).Int("ancode", ancode).Msg("cannot remove nta room-alone waiver")
		return false, err
	}
	return res.DeletedCount > 0, nil
}
