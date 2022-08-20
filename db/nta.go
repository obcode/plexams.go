package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
)

func (db *DB) AddNta(ctx context.Context, nta *model.NTA) (*model.NTA, error) {
	collection := db.Client.Database("plexams").Collection("nta")

	_, err := collection.InsertOne(ctx, nta)
	if err != nil {
		log.Error().Err(err).Interface("nta", nta).Msg("cannot insert nta into DB")
		return nil, err
	}

	return nta, nil // FIXME: return NTA from DB?
}

func (db *DB) Ntas(ctx context.Context) ([]*model.NTA, error) {
	collection := db.Client.Database("plexams").Collection("nta")
	ntas := make([]*model.NTA, 0)

	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Str("collection", "nta").Msg("MongoDB Find")
		return ntas, err
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var nta model.NTA

		err := cur.Decode(&nta)
		if err != nil {
			log.Error().Err(err).Str("collection", "nta").Interface("cur", cur).
				Msg("Cannot decode to nta")
			return ntas, err
		}

		ntas = append(ntas, &nta)
	}

	if err := cur.Err(); err != nil {
		log.Error().Err(err).Str("collection", "nta").Msg("Cursor returned error")
		return ntas, err
	}

	return ntas, nil
}
