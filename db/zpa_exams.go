package db

import (
	"context"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	collecttionAll      = "zpaexams"
	collectionToPlan    = "zpaexams-to-plan"
	collectionNotToPlan = "zpaexams-not-to-plan"
)

func (db *DB) GetZPAExamsToPlan(ctx context.Context) ([]*model.ZPAExam, error) {
	return db.getZPAExams(ctx, collectionToPlan)
}

func (db *DB) GetZPAExamsNotToPlan(ctx context.Context) ([]*model.ZPAExam, error) {
	return db.getZPAExams(ctx, collectionNotToPlan)
}

func (db *DB) GetZPAExams(ctx context.Context) ([]*model.ZPAExam, error) {
	return db.getZPAExams(ctx, collecttionAll)
}

func (db *DB) getZPAExams(ctx context.Context, fromCollection string) ([]*model.ZPAExam, error) {
	collection := db.Client.Database(databaseName(db.semester)).Collection(fromCollection)

	exams := make([]*model.ZPAExam, 0)

	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("collection", "zpaexams").Msg("MongoDB Find")
		return exams, err
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var exam model.ZPAExam

		err := cur.Decode(&exam)
		if err != nil {
			log.Error().Err(err).Str("semester", db.semester).Str("collection", "zpaexams").Interface("cur", cur).
				Msg("Cannot decode to customer")
			return exams, err
		}

		exams = append(exams, &exam)
	}

	if err := cur.Err(); err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("collection", "zpaexams").Msg("Cursor returned error")
		return exams, err
	}

	return exams, nil
}

func (db *DB) GetZpaExamByAncode(ctx context.Context, anCode int) (*model.ZPAExam, error) {
	collection := db.Client.Database(databaseName(db.semester)).Collection("zpaexams")

	var result model.ZPAExam
	err := collection.FindOne(ctx, bson.D{{Key: "ancode", Value: anCode}}).Decode(&result)
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).
			Int("anCode", anCode).Msg("cannot find ZPA exam")
		return nil, err
	}

	return &result, nil
}

func (db *DB) CacheZPAExams(exams []*model.ZPAExam) error {
	collection := db.Client.Database(databaseName(db.semester)).Collection("zpaexams")

	examsIntf := make([]interface{}, 0, len(exams))

	for _, v := range exams {
		examsIntf = append(examsIntf, v)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := collection.Drop(ctx)
	if err != nil {
		return err
	}

	res, err := collection.InsertMany(ctx, examsIntf)
	if err != nil {
		return err
	}

	log.Debug().Str("semester", db.semester).Int("documents", len(res.InsertedIDs)).Msg("inserted zpaexams")

	return nil
}

func (db *DB) SetZPAExamsToPlan(ctx context.Context, exams []*model.ZPAExam) error {
	return db.setZPAExams(ctx, exams, collectionToPlan)
}

func (db *DB) SetZPAExamsNotToPlan(ctx context.Context, exams []*model.ZPAExam) error {
	return db.setZPAExams(ctx, exams, collectionNotToPlan)
}

func (db *DB) setZPAExams(ctx context.Context, exams []*model.ZPAExam, toCollection string) error {
	collection := db.Client.Database(databaseName(db.semester)).Collection(toCollection)

	examsIntf := make([]interface{}, 0, len(exams))

	for _, v := range exams {
		examsIntf = append(examsIntf, v)
	}

	err := collection.Drop(ctx)
	if err != nil {
		return err
	}

	res, err := collection.InsertMany(ctx, examsIntf)
	if err != nil {
		return err
	}

	log.Debug().Str("semester", db.semester).Str("collection", toCollection).
		Int("documents", len(res.InsertedIDs)).Msg("inserted zpaexams")

	return nil
}
