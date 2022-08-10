package db

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
)

func (db *DB) GetPrimussStudentRegsForAncode(ctx context.Context, program string, anCode int) ([]*model.StudentReg, error) {
	studentRegs, err := db.getPrimussStudentRegs(ctx, program)
	if err != nil {
		return nil, err
	}

	return studentRegs[anCode], nil
}

func (db *DB) getPrimussStudentRegs(ctx context.Context, program string) (map[int][]*model.StudentReg, error) {
	collection := db.getCollection(program, StudentRegs)

	studentRegs := make(map[int][]*model.StudentReg)

	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("program", program).Msg("MongoDB Find (studentregs)")
		return studentRegs, err
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var studentReg model.StudentReg

		err := cur.Decode(&studentReg)
		if err != nil {
			log.Error().Err(err).Str("semester", db.semester).Str("program", program).Interface("cur", cur).
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
		if !db.checkStudentRegsCount(ctx, program, k, len(v)) {
			return nil, fmt.Errorf("problem with studentregs, ancode = %d, #studentregs = %d", k, len(v))
		}
	}

	if err := cur.Err(); err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("program", program).Msg("Cursor returned error")
		return studentRegs, err
	}

	return studentRegs, nil
}

type Count struct {
	AnCode int `bson:"AnCo"`
	Sum    int `bson:"Sum"`
}

func (db *DB) checkStudentRegsCount(ctx context.Context, program string, anCode, studentRegsCount int) bool {
	// log.Debug().Str("collectionName", collectionName).Int("anCode", anCode).Int("studentRegsCount", studentRegsCount).
	// 	Msg("checking count")
	collection := db.getCollection(program, Counts)
	var result Count
	err := collection.FindOne(ctx, bson.D{{Key: "AnCo", Value: anCode}}).Decode(&result)
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("program", program).
			Int("anCode", anCode).Int("studentRegsCount", studentRegsCount).Msg("error finding count")
		return false
	}
	if result.Sum != studentRegsCount {
		log.Debug().Str("semester", db.semester).Str("program", program).
			Int("anCode", anCode).Int("studentRegsCount", studentRegsCount).Int("result.Sum", result.Sum).
			Msg("sum != student registrations")
		return false
	}
	return true
}
