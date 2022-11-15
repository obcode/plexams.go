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
	collectionAll       = "zpaexams"
	collectionToPlan    = "zpaexams-to-plan"
	collectionNotToPlan = "zpaexams-not-to-plan"
)

func (db *DB) GetZPAExamsToPlan(ctx context.Context) ([]*model.ZPAExam, error) {
	ancodes, err := db.getZpaAnCodesFromCollection(ctx, collectionToPlan)
	if err != nil {
		return nil, err
	}

	exams := make([]*model.ZPAExam, 0)

	for _, ancode := range ancodes {
		exam, err := db.GetZpaExamByAncode(ctx, ancode.Ancode)
		if err != nil {
			log.Error().Err(err).Str("semester", db.semester).
				Int("ancode", ancode.Ancode).Msg("zpa exam with ancode not found")
		} else {
			exams = append(exams, exam)
		}
	}

	return exams, nil
}

func (db *DB) GetZPAExamsNotToPlan(ctx context.Context) ([]*model.ZPAExam, error) {
	ancodes, err := db.getZpaAnCodesFromCollection(ctx, collectionNotToPlan)
	if err != nil {
		return nil, err
	}

	exams := make([]*model.ZPAExam, 0)

	for _, ancode := range ancodes {
		exam, err := db.GetZpaExamByAncode(ctx, ancode.Ancode)
		if err != nil {
			log.Error().Err(err).Str("semester", db.semester).
				Int("ancode", ancode.Ancode).Msg("zpa exam with ancode not found")
		} else {
			exams = append(exams, exam)
		}
	}

	return exams, nil
}

func (db *DB) GetZpaAncodesPlanned(ctx context.Context) ([]*model.AnCode, error) {
	return db.getZpaAnCodesFromCollection(ctx, collectionToPlan)
}

func (db *DB) getZpaAnCodesFromCollection(ctx context.Context, collectionName string) ([]*model.AnCode, error) {
	collection := db.Client.Database(databaseName(db.semester)).Collection(collectionName)
	ancodes := make([]*model.AnCode, 0)
	findOptions := options.Find()
	findOptions.SetSort(bson.D{{Key: "ancode", Value: 1}})

	cur, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("collection", collectionName).Msg("MongoDB Find")
		return ancodes, err
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var ancode model.AnCode

		err := cur.Decode(&ancode)
		if err != nil {
			log.Error().Err(err).Str("semester", db.semester).Str("collection", collectionName).Interface("cur", cur).
				Msg("Cannot decode to exam")
			return ancodes, err
		}

		ancodes = append(ancodes, &ancode)
	}

	if err := cur.Err(); err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("collection", collectionName).Msg("Cursor returned error")
		return ancodes, err
	}

	return ancodes, nil
}

func (db *DB) GetZPAExams(ctx context.Context) ([]*model.ZPAExam, error) {
	collection := db.Client.Database(databaseName(db.semester)).Collection(collectionAll)

	exams := make([]*model.ZPAExam, 0)

	findOptions := options.Find()
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
				Msg("Cannot decode to exam")
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

func (db *DB) GetZpaExamByAncode(ctx context.Context, ancode int) (*model.ZPAExam, error) {
	collection := db.Client.Database(databaseName(db.semester)).Collection("zpaexams")

	var result model.ZPAExam
	err := collection.FindOne(ctx, bson.D{{Key: "ancode", Value: ancode}}).Decode(&result)
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).
			Int("ancode", ancode).Msg("cannot find ZPA exam")
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

func (db *DB) AddZpaExamToPlan(ctx context.Context, ancode int, unknown bool) (bool, error) {
	exam, err := db.GetZpaExamByAncode(ctx, ancode)
	if err != nil {
		return false, err
	}

	// maybe rm exam from not to plan
	if !unknown {
		collectionNot := db.Client.Database(databaseName(db.semester)).Collection(collectionNotToPlan)

		res, err := collectionNot.DeleteOne(ctx, bson.D{{Key: "ancode", Value: ancode}})
		if err != nil {
			log.Error().Err(err).Str("semester", db.semester).
				Int("ancode", ancode).Msg("cannot remove ZPA exam from not planned exams")
			return false, err
		}
		if res.DeletedCount != 1 {
			log.Error().Err(err).Str("semester", db.semester).
				Int("ancode", ancode).Int64("deletedCount", res.DeletedCount).Msg("not removed exactly one ZPA exam from not planned exams")
		}
	}

	// add exam to to plan
	collectionTo := db.Client.Database(databaseName(db.semester)).Collection(collectionToPlan)

	_, err = collectionTo.InsertOne(ctx, exam)
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).
			Int("ancode", ancode).Msg("cannot add ZPA exam to planned exams")
		return false, err
	}

	return true, nil
}

func (db *DB) RmZpaExamFromPlan(ctx context.Context, ancode int, unknown bool) (bool, error) {
	exam, err := db.GetZpaExamByAncode(ctx, ancode)
	if err != nil {
		return false, err
	}

	// maybe rm exam from not to plan
	if !unknown {
		collectionTo := db.Client.Database(databaseName(db.semester)).Collection(collectionToPlan)

		res, err := collectionTo.DeleteOne(ctx, bson.D{{Key: "ancode", Value: ancode}})
		if err != nil {
			log.Error().Err(err).Str("semester", db.semester).
				Int("ancode", ancode).Msg("cannot remove ZPA exam from planned exams")
			return false, err
		}
		if res.DeletedCount != 1 {
			log.Error().Err(err).Str("semester", db.semester).
				Int("ancode", ancode).Int64("deletedCount", res.DeletedCount).Msg("not removed exactly one ZPA exam from planned exams")
		}
	}

	// add exam to to plan
	collectionNot := db.Client.Database(databaseName(db.semester)).Collection(collectionNotToPlan)

	_, err = collectionNot.InsertOne(ctx, exam)
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).
			Int("ancode", ancode).Msg("cannot add ZPA exam to not planned exams")
		return false, err
	}

	return true, nil
}
