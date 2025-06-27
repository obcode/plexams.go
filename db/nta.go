package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (db *DB) AddNta(ctx context.Context, nta *model.NTA) (*model.NTA, error) {
	collection := db.Client.Database("plexams").Collection(collectionNameNTAs)

	_, err := collection.InsertOne(ctx, nta)
	if err != nil {
		log.Error().Err(err).Interface("nta", nta).Msg("cannot insert nta into DB")
		return nil, err
	}

	return db.Nta(ctx, nta.Mtknr)
}

func (db *DB) Nta(ctx context.Context, mtknr string) (*model.NTA, error) {
	collection := db.Client.Database("plexams").Collection(collectionNameNTAs)

	res := collection.FindOne(ctx, bson.D{{Key: "mtknr", Value: mtknr}})
	if res.Err() == mongo.ErrNoDocuments {
		return nil, nil
	}

	var nta *model.NTA

	err := res.Decode(&nta)
	if err != nil {
		log.Error().Err(res.Err()).Str("mtknr", mtknr).Msg("error while finding nta")
		return nil, err
	}

	return nta, nil
}

func (db *DB) Ntas(ctx context.Context) ([]*model.NTA, error) {
	collection := db.Client.Database("plexams").Collection(collectionNameNTAs)

	findOptions := options.Find()
	findOptions.SetSort(bson.D{{Key: "name", Value: 1}})

	cur, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionNameNTAs).Msg("MongoDB Find")
		return nil, err
	}
	defer cur.Close(ctx) //nolint:errcheck

	ntas := make([]*model.NTA, 0)
	err = cur.All(ctx, &ntas)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionNameNTAs).Msg("Cannot decode to ntas")
		return nil, err
	}

	return ntas, nil
}

func (db *DB) SetSemesterOnNTAs(ctx context.Context, studentRegs []interface{}) error {
	collection := db.Client.Database("plexams").Collection(collectionNameNTAs)

	for _, regRaw := range studentRegs {
		reg := regRaw.(*model.Student)
		if reg.Nta == nil {
			continue
		}

		res := collection.FindOneAndUpdate(ctx, bson.D{{Key: "mtknr", Value: reg.Mtknr}},
			bson.M{"$set": bson.M{"lastSemester": db.semester}})

		if res.Err() != nil {
			if res.Err() == mongo.ErrNoDocuments {
				log.Error().Err(res.Err()).Str("mtknr", reg.Mtknr).Msg("nta with mtknr not found")
			} else {
				log.Error().Err(res.Err()).Str("mtknr", reg.Mtknr).Msg("error when setting semester on nta")
				return res.Err()
			}
		} else {
			log.Debug().Str("mtknr", reg.Mtknr).Str("last semester", db.semester).Msg("last semester set on nta")
		}
	}

	return nil
}

func (db *DB) NtasWithRegs(ctx context.Context) ([]*model.Student, error) {
	collection := db.Client.Database(db.databaseName).Collection(collectionStudentRegsPerStudentPlanned)

	findOptions := options.Find()
	findOptions.SetSort(bson.D{{Key: "nta.name", Value: 1}})

	cur, err := collection.Find(ctx, bson.D{{Key: "nta", Value: bson.D{{Key: "$ne", Value: nil}}}}, findOptions)
	if err != nil {
		log.Error().Err(err).Str("collection", "nta").Msg("MongoDB Find")
		return nil, err
	}
	defer cur.Close(ctx) //nolint:errcheck

	ntas := make([]*model.Student, 0)
	err = cur.All(ctx, &ntas)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionNameNTAs).Msg("Cannot decode to rooms")
		return nil, err
	}

	return ntas, nil
}

func (db *DB) NtaWithRegs(ctx context.Context, mtknr string) (*model.NTAWithRegs, error) {
	collection := db.Client.Database(db.databaseName).Collection(collectionNameNTAs)

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
