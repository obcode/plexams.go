package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func (db *DB) GetPrimussExamsForAncode(ctx context.Context, ancode int) ([]*model.PrimussExam, error) {
	programs, err := db.GetPrograms(ctx)
	if err != nil {
		return nil, err
	}

	exams := make([]*model.PrimussExam, 0)
	for _, program := range programs {
		exam, err := db.GetPrimussExam(ctx, program, ancode)
		if err != nil {
			log.Error().Err(err).Str("semester", db.semester).Str("program", program).
				Int("ancode", ancode).Msg("cannot find primuss exam")
		} else {
			exams = append(exams, exam)
		}
	}

	return exams, nil
}

func (db *DB) GetPrimussExam(ctx context.Context, program string, ancode int) (*model.PrimussExam, error) {
	collection := db.getCollection(program, Exams)

	var exam model.PrimussExam
	err := collection.FindOne(ctx, bson.D{{Key: "AnCode", Value: ancode}}).Decode(&exam)
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("program", program).
			Int("ancode", ancode).Msg("cannot find primuss exam")
		return nil, err
	}

	return &exam, nil
}

func (db *DB) PrimussExamExists(ctx context.Context, program string, ancode int) (bool, error) {
	collection := db.getCollection(program, Exams)

	err := collection.FindOne(ctx, bson.D{{Key: "AnCode", Value: ancode}}).Err()
	if err == mongo.ErrNoDocuments {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	return true, nil
}

func (db *DB) ChangeAncode(ctx context.Context, program string, ancode, newAncode int) (*model.PrimussExam, error) {
	collection := db.getCollection(program, Exams)

	filter := bson.D{{Key: "AnCode", Value: ancode}}
	update := bson.D{{Key: "$set", Value: bson.D{{Key: "AnCode", Value: newAncode}}}}

	result, err := collection.UpdateOne(ctx, filter, update)

	if err != nil {
		log.Error().Err(err).
			Str("program", program).Int("from", ancode).Int("to", newAncode).
			Msg("error while trying to change ancode.")
		return nil, err
	}

	if result.MatchedCount == 0 {
		log.Debug().
			Str("program", program).Int("from", ancode).Int("to", newAncode).
			Msg("no exam updated while trying to change ancode.")
	}

	return db.GetPrimussExam(ctx, program, newAncode)
}

func (db *DB) GetPrimussExams(ctx context.Context) ([]*model.PrimussExamByProgram, error) {
	programs, err := db.GetPrograms(ctx)

	primussExams := make([]*model.PrimussExamByProgram, 0)
	for _, program := range programs {
		exams, err := db.getPrimussExams(ctx, program)
		if err != nil {
			return primussExams, err
		}
		primussExams = append(primussExams, &model.PrimussExamByProgram{
			Program: program,
			Exams:   exams,
		})
	}

	return primussExams, err
}

func (db *DB) getPrimussExams(ctx context.Context, program string) ([]*model.PrimussExam, error) {
	collection := db.getCollection(program, Exams)

	exams := make([]*model.PrimussExam, 0)

	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("program", program).Msg("MongoDB Find")
		return exams, err
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var exam model.PrimussExam

		err := cur.Decode(&exam)
		if err != nil {
			log.Error().Err(err).Str("semester", db.semester).Str("program", program).Interface("cur", cur).
				Msg("Cannot decode to exam")
			return exams, err
		}

		exams = append(exams, &exam)
	}

	if err := cur.Err(); err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("program", program).Msg("Cursor returned error")
		return exams, err
	}

	return exams, nil
}
