package db

import (
	"context"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
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

	findOptions := options.Find()
	// Sort by `price` field descending
	findOptions.SetSort(bson.D{{Key: "ancode", Value: 1}})

	cur, err := collection.Find(ctx, bson.M{}, findOptions)
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

func (db *DB) AddZpaExamToPlan(ctx context.Context, anCode int) (bool, error) {
	exam, err := db.GetZpaExamByAncode(ctx, anCode)
	if err != nil {
		return false, err
	}
	// rm exam from not to plan
	collectionNot := db.Client.Database(databaseName(db.semester)).Collection(collectionNotToPlan)

	res, err := collectionNot.DeleteOne(ctx, bson.D{{Key: "ancode", Value: anCode}})
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).
			Int("anCode", anCode).Msg("cannot remove ZPA exam from not planned exams")
		return false, err
	}
	if res.DeletedCount != 1 {
		log.Error().Err(err).Str("semester", db.semester).
			Int("anCode", anCode).Int64("deletedCount", res.DeletedCount).Msg("not removed exactly one ZPA exam from not planned exams")
	}

	// add exam to to plan
	collectionTo := db.Client.Database(databaseName(db.semester)).Collection(collectionToPlan)

	_, err = collectionTo.InsertOne(ctx, exam)
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).
			Int("anCode", anCode).Msg("cannot add ZPA exam to planned exams")
		return false, err
	}

	return true, nil
}

func (db *DB) RmZpaExamFromPlan(ctx context.Context, anCode int) (bool, error) {
	exam, err := db.GetZpaExamByAncode(ctx, anCode)
	if err != nil {
		return false, err
	}
	// rm exam from not to plan
	collectionTo := db.Client.Database(databaseName(db.semester)).Collection(collectionToPlan)

	res, err := collectionTo.DeleteOne(ctx, bson.D{{Key: "ancode", Value: anCode}})
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).
			Int("anCode", anCode).Msg("cannot remove ZPA exam from planned exams")
		return false, err
	}
	if res.DeletedCount != 1 {
		log.Error().Err(err).Str("semester", db.semester).
			Int("anCode", anCode).Int64("deletedCount", res.DeletedCount).Msg("not removed exactly one ZPA exam from planned exams")
	}

	// add exam to to plan
	collectionNot := db.Client.Database(databaseName(db.semester)).Collection(collectionNotToPlan)

	_, err = collectionNot.InsertOne(ctx, exam)
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).
			Int("anCode", anCode).Msg("cannot add ZPA exam to not planned exams")
		return false, err
	}

	return true, nil
}
