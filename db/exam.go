package db

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (db *DB) AddAdditionalExam(ctx context.Context, exam model.AdditionalExamInput) (bool, error) {
	collection := db.Client.Database(databaseName(db.semester)).Collection(collectionNameAdditionalExams)
	_, err := collection.InsertOne(ctx, exam)
	if err != nil {
		return false, err
	}

	return true, nil
}

func (db *DB) AdditionalExams(ctx context.Context) ([]*model.AdditionalExam, error) {
	collection := db.Client.Database(databaseName(db.semester)).Collection(collectionNameAdditionalExams)

	exams := make([]*model.AdditionalExam, 0)

	findOptions := options.Find()
	findOptions.SetSort(bson.D{{Key: "ancode", Value: 1}})

	cur, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("collection", collectionNameAdditionalExams).Msg("MongoDB Find")
		return exams, err
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var exam model.AdditionalExam

		err := cur.Decode(&exam)
		if err != nil {
			log.Error().Err(err).Str("semester", db.semester).Str("collection", collectionNameAdditionalExams).Interface("cur", cur).
				Msg("Cannot decode to additional exam")
			return exams, err
		}

		exams = append(exams, &exam)
	}

	if err := cur.Err(); err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("collection", collectionNameAdditionalExams).Msg("Cursor returned error")
		return exams, err
	}

	return exams, nil
}

func (db *DB) ExamsAlreadyPrepared(ctx context.Context) bool {
	collection := db.Client.Database(databaseName(db.semester)).Collection("exams")

	docsCount, err := collection.CountDocuments(ctx, bson.D{})
	if err != nil {
		log.Error().Err(err).Msg("cannot count exams in db")
	}

	return docsCount != 0
}

func (db *DB) AddExam(ctx context.Context, exam *model.Exam) error {
	collection := db.Client.Database(databaseName(db.semester)).Collection("exams")

	result := collection.FindOne(ctx, bson.D{{Key: "anCode", Value: exam.AnCode}})
	if result.Err() == nil {
		log.Error().Int("anCode", exam.AnCode).Msg("cannot add exam, exam with ancode already in db")
		return fmt.Errorf("cannot add exam, exam with ancode %d already in db", exam.AnCode)
	}

	if result.Err() == mongo.ErrNoDocuments {
		_, err := collection.InsertOne(ctx, exam)
		if err != nil {
			return err
		}
		return nil
	}

	return result.Err()
}

func (db *DB) SaveConnectedExams(ctx context.Context, exams []*model.ConnectedExam) error {
	collection := db.Client.Database(databaseName(db.semester)).Collection(collectionNameConnectedExams)

	err := collection.Drop(ctx)
	if err != nil {
		log.Error().Err(err).
			Str("collectionName", collectionNameConnectedExams).
			Msg("cannot drop collection")
		return err
	}

	examsToInsert := make([]interface{}, 0, len(exams))
	for _, exam := range exams {
		examsToInsert = append(examsToInsert, exam)
	}

	_, err = collection.InsertMany(ctx, examsToInsert)
	if err != nil {
		log.Error().Err(err).
			Str("collectionName", collectionNameConnectedExams).
			Msg("cannot insert exams")
		return err
	}

	return nil
}

func (db *DB) GetConnectedExams(ctx context.Context) ([]*model.ConnectedExam, error) {
	collection := db.Client.Database(databaseName(db.semester)).Collection(collectionNameConnectedExams)

	exams := make([]*model.ConnectedExam, 0)

	findOptions := options.Find()
	findOptions.SetSort(bson.D{{Key: "zpaexam.ancode", Value: 1}})

	cur, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("collection", collectionNameConnectedExams).Msg("MongoDB Find")
		return exams, err
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var exam model.ConnectedExam

		err := cur.Decode(&exam)
		if err != nil {
			log.Error().Err(err).Str("semester", db.semester).Str("collection", collectionNameConnectedExams).Interface("cur", cur).
				Msg("Cannot decode to additional exam")
			return exams, err
		}

		exams = append(exams, &exam)
	}

	if err := cur.Err(); err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("collection", collectionNameConnectedExams).Msg("Cursor returned error")
		return exams, err
	}

	return exams, nil
}

func (db *DB) SaveExamsWithRegs(ctx context.Context, exams []*model.ExamWithRegs) error {
	collection := db.Client.Database(databaseName(db.semester)).Collection(collectionNameExamsWithRegs)

	err := collection.Drop(ctx)
	if err != nil {
		log.Error().Err(err).
			Str("collectionName", collectionNameExamsWithRegs).
			Msg("cannot drop collection")
		return err
	}

	examsToInsert := make([]interface{}, 0, len(exams))
	for _, exam := range exams {
		examsToInsert = append(examsToInsert, exam)
	}

	_, err = collection.InsertMany(ctx, examsToInsert)
	if err != nil {
		log.Error().Err(err).
			Str("collectionName", collectionNameExamsWithRegs).
			Msg("cannot insert exams")
		return err
	}

	return nil
}
