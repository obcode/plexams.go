package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
)

func (db *DB) AddNonZpaExam(ctx context.Context, exam *model.ZPAExam) error {
	_, err := db.Client.Database(db.databaseName).Collection(collectionNonZpaExams).InsertOne(ctx, exam)
	return err
}

func (db *DB) NonZpaExam(ctx context.Context, ancode int) (*model.ZPAExam, error) {
	var exam model.ZPAExam
	err := db.Client.Database(db.databaseName).Collection(collectionNonZpaExams).FindOne(ctx, bson.M{"ancode": ancode}).Decode(&exam)
	if err != nil {
		log.Error().Err(err).Msg("cannot get non zpa exam")
		return nil, err
	}
	return &exam, nil
}

func (db *DB) NonZpaExams(ctx context.Context) ([]*model.ZPAExam, error) {
	var exams []*model.ZPAExam
	cur, err := db.Client.Database(db.databaseName).Collection(collectionNonZpaExams).Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Msg("cannot get non zpa exams")
		return nil, err
	}
	defer cur.Close(ctx) //nolint:errcheck

	err = cur.All(ctx, &exams)
	if err != nil {
		log.Error().Err(err).Msg("cannot get non zpa exams")
		return nil, err
	}
	return exams, nil
}
