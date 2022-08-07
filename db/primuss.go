package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func (db *DB) GetPrimussExams(ctx context.Context) ([]*model.PrimussExamByProgram, error) {
	collections, err := db.Client.Database(databaseName(db.semester)).ListCollectionNames(ctx,
		bson.D{primitive.E{
			Key: "name",
			Value: bson.D{
				primitive.E{Key: "$regex",
					Value: primitive.Regex{Pattern: "exams_"},
				},
			},
		}})

	primussExams := make([]*model.PrimussExamByProgram, 0)
	for _, collectionName := range collections {
		exams, err := db.getPrimussExams(ctx, collectionName)
		if err != nil {
			return primussExams, err
		}
		primussExams = append(primussExams, &model.PrimussExamByProgram{
			Program: strings.Replace(collectionName, "exams_", "", 1),
			Exams:   exams,
		})
	}

	return primussExams, err
}

func (db *DB) getPrimussExams(ctx context.Context, collectionName string) ([]*model.PrimussExam, error) {
	collection := db.Client.Database(databaseName(db.semester)).Collection(collectionName)

	exams := make([]*model.PrimussExam, 0)

	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("collection", collectionName).Msg("MongoDB Find")
		return exams, err
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var exam model.PrimussExam

		err := cur.Decode(&exam)
		if err != nil {
			log.Error().Err(err).Str("semester", db.semester).Str("collection", collectionName).Interface("cur", cur).
				Msg("Cannot decode to exam")
			return exams, err
		}

		exams = append(exams, &exam)
	}

	if err := cur.Err(); err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("collection", collectionName).Msg("Cursor returned error")
		return exams, err
	}

	return exams, nil
}

func (db *DB) GetPrimussStudentRegsForAncode(ctx context.Context, program string, anCode int) ([]*model.StudentReg, error) {
	studentRegs, err := db.getPrimussStudentRegs(ctx, fmt.Sprintf("studentregs_%s", program))
	if err != nil {
		return nil, err
	}

	return studentRegs[anCode], nil
}

func (db *DB) getPrimussStudentRegs(ctx context.Context, collectionName string) (map[int][]*model.StudentReg, error) {
	collection := db.Client.Database(databaseName(db.semester)).Collection(collectionName)

	studentRegs := make(map[int][]*model.StudentReg)

	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("collection", collectionName).Msg("MongoDB Find")
		return studentRegs, err
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var studentReg model.StudentReg

		err := cur.Decode(&studentReg)
		if err != nil {
			log.Error().Err(err).Str("semester", db.semester).Str("collection", collectionName).Interface("cur", cur).
				Msg("Cannot decode to exam")
			return studentRegs, err
		}

		regs, ok := studentRegs[studentReg.AnCode]
		if !ok {
			regs = make([]*model.StudentReg, 0)
		}

		studentRegs[studentReg.AnCode] = append(regs, &studentReg)

	}

	for k, v := range studentRegs {
		if !db.checkStudentRegsCount(ctx, collectionName, k, len(v)) {
			return nil, fmt.Errorf("problem with studentregs, ancode = %d, #studentregs = %d", k, len(v))
		}
	}

	if err := cur.Err(); err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("collection", collectionName).Msg("Cursor returned error")
		return studentRegs, err
	}

	return studentRegs, nil
}

type Count struct {
	AnCode int `bson:"AnCo"`
	Sum    int `bson:"Sum"`
}

func (db *DB) checkStudentRegsCount(ctx context.Context, collectionName string, anCode, studentRegsCount int) bool {
	// log.Debug().Str("collectionName", collectionName).Int("anCode", anCode).Int("studentRegsCount", studentRegsCount).
	// 	Msg("checking count")
	collectionNameCount := strings.Replace(collectionName, "studentregs", "count", 1)
	collection := db.Client.Database(databaseName(db.semester)).Collection(collectionNameCount)
	var result Count
	err := collection.FindOne(ctx, bson.D{{Key: "AnCo", Value: anCode}}).Decode(&result)
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("collection", collectionNameCount).
			Int("anCode", anCode).Int("studentRegsCount", studentRegsCount).Msg("error finding count")
		return false
	}
	if result.Sum != studentRegsCount {
		log.Debug().Str("semester", db.semester).Str("collection", collectionNameCount).
			Int("anCode", anCode).Int("studentRegsCount", studentRegsCount).Int("result.Sum", result.Sum).
			Msg("sum != student registrations")
		return false
	}
	return true
}
