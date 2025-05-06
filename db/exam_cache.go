package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (db *DB) CacheExam(ctx context.Context, exam *model.Exam) error {
	collection := db.Client.Database(db.databaseName).Collection(collectionCachedExams)

	res, err := collection.ReplaceOne(ctx, bson.D{{Key: "ancode", Value: exam.Ancode}}, exam, options.Replace().SetUpsert(true))

	log.Debug().Interface("res", res).Msg("replaced")

	return err
}

func (db *DB) CachedExam(ctx context.Context, ancode int) (*model.Exam, error) {
	collection := db.Client.Database(db.databaseName).Collection(collectionCachedExams)

	res := collection.FindOne(ctx, bson.D{{Key: "ancode", Value: ancode}})
	if res.Err() == mongo.ErrNoDocuments {
		return nil, nil
	}

	var exam model.Exam

	err := res.Decode(&exam)

	return &exam, err
}

func (db *DB) CachedExams(ctx context.Context) ([]*model.Exam, error) {
	collection := db.Client.Database(db.databaseName).Collection(collectionCachedExams)

	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Msg("cannot get cached exams")
		return nil, err
	}
	defer cur.Close(ctx) //nolint:errcheck

	exams := make([]*model.Exam, 0)

	err = cur.All(ctx, &exams)
	if err != nil {
		log.Error().Err(err).Msg("cannot decode cached exams")
		return nil, err
	}

	return exams, nil
}

func (db *DB) RmCacheExams(ctx context.Context) error {
	return db.Client.Database(db.databaseName).Collection(collectionCachedExams).Drop(ctx)
}
