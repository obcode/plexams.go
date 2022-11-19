package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
)

const collectionNameNTAs = "nta"

func (db *DB) AddNta(ctx context.Context, nta *model.NTA) (*model.NTA, error) {
	collection := db.Client.Database("plexams").Collection(collectionNameNTAs)

	_, err := collection.InsertOne(ctx, nta)
	if err != nil {
		log.Error().Err(err).Interface("nta", nta).Msg("cannot insert nta into DB")
		return nil, err
	}

	return nta, nil // FIXME: return NTA from DB?
}

func (db *DB) Ntas(ctx context.Context) ([]*model.NTA, error) {
	collection := db.Client.Database("plexams").Collection(collectionNameNTAs)
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

func (db *DB) NtasWithRegs(ctx context.Context) ([]*model.NTAWithRegs, error) {
	collection := db.Client.Database(databaseName(db.semester)).Collection(collectionNameNTAs)
	ntas := make([]*model.NTAWithRegs, 0)

	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Str("collection", "nta").Msg("MongoDB Find")
		return ntas, err
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var nta model.NTAWithRegs

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

func (db *DB) Nta(ctx context.Context, mtknr string) (*model.NTAWithRegs, error) {
	collection := db.Client.Database(databaseName(db.semester)).Collection(collectionNameNTAs)

	var nta model.NTAWithRegs
	res := collection.FindOne(ctx, bson.D{{Key: "nta.mtknr", Value: mtknr}})
	if res.Err() != nil {
		log.Error().Err(res.Err()).Str("mtknr", mtknr).Msg("no nta found")
		return nil, nil // no constraint available
	}
	err := res.Decode(&nta)
	if err != nil {
		log.Error().Err(res.Err()).Str("mtknr", mtknr).Msg("cannot decode constraint")
		return nil, err
	}

	return &nta, nil
}

func (db *DB) SaveSemesterNTAs(ctx context.Context, ntaWithRegs []*model.NTAWithRegs) error {
	collection := db.Client.Database(databaseName(db.semester)).Collection(collectionNameNTAs)

	err := collection.Drop(ctx)
	if err != nil {
		log.Error().Err(err).
			Str("collectionName", collectionNameNTAs).
			Msg("cannot drop collection")
		return err
	}

	ntaWithRegsToInsert := make([]interface{}, 0, len(ntaWithRegs))
	for _, ntaWithReg := range ntaWithRegs {
		ntaWithRegsToInsert = append(ntaWithRegsToInsert, ntaWithReg)
	}

	_, err = collection.InsertMany(ctx, ntaWithRegsToInsert)
	if err != nil {
		log.Error().Err(err).
			Str("collectionName", collectionNameNTAs).
			Msg("cannot insert exams")
		return err
	}

	for _, nta := range ntaWithRegs {
		err := db.setCurrentSemesterOnNTA(ctx, nta.Nta.Mtknr)
		if err != nil {
			return err
		}
	}

	return nil
}

func (db *DB) setCurrentSemesterOnNTA(ctx context.Context, mtknr string) error {
	collection := db.Client.Database("plexams").Collection(collectionNameNTAs)

	filter := bson.D{{Key: "mtknr", Value: mtknr}}
	update := bson.D{{Key: "$set", Value: bson.D{{Key: "lastSemester", Value: db.semester}}}}

	_, err := collection.UpdateOne(ctx, filter, update)
	if err != nil {
		log.Error().Err(err).Str("mtknr", mtknr).
			Str("collectionName", collectionNameNTAs).
			Msg("cannot update nta with current semester")
		return err
	}
	return nil
}
