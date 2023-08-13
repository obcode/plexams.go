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

	return nta, nil // FIXME: return NTA from DB?
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
	defer cur.Close(ctx)

	ntas := make([]*model.NTA, 0)
	err = cur.All(ctx, &ntas)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionNameNTAs).Msg("Cannot decode to ntas")
		return nil, err
	}

	return ntas, nil
}

// // Deprecated: remove me
func (db *DB) NtasWithRegs(ctx context.Context) ([]*model.NTAWithRegs, error) {
	collection := db.Client.Database(db.databaseName).Collection(collectionNameNTAs)

	findOptions := options.Find()
	findOptions.SetSort(bson.D{{Key: "nta.name", Value: 1}})

	cur, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		log.Error().Err(err).Str("collection", "nta").Msg("MongoDB Find")
		return nil, err
	}
	defer cur.Close(ctx)

	ntas := make([]*model.NTAWithRegs, 0)
	err = cur.All(ctx, &ntas)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionNameNTAs).Msg("Cannot decode to rooms")
		return nil, err
	}

	return ntas, nil
}

// // Deprecated: remove me
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

// // Deprecated: remove me
func (db *DB) SaveSemesterNTAs(ctx context.Context, ntaWithRegs []*model.NTAWithRegs) error {
	collection := db.Client.Database(db.databaseName).Collection(collectionNameNTAs)

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
			Msg("cannot insert ntas")
		return err
	}

	for _, nta := range ntaWithRegs {
		err := db.SetCurrentSemesterOnNTA(ctx, nta.Nta.Mtknr)
		if err != nil {
			return err
		}
	}

	return nil
}

// TODO: when to call?
func (db *DB) SetCurrentSemesterOnNTA(ctx context.Context, mtknr string) error {
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
