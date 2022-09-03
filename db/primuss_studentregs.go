package db

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
)

func (db *DB) GetPrimussStudentRegsForProgrammAncode(ctx context.Context, program string, anCode int) ([]*model.StudentReg, error) {
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

func (db *DB) StudentRegsForProgram(ctx context.Context, program string) ([]*model.StudentReg, error) {
	collection := db.getCollection(program, StudentRegs)

	studentRegs := make([]*model.StudentReg, 0)

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

		studentRegs = append(studentRegs, &studentReg)
	}

	if err := cur.Err(); err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("program", program).Msg("Cursor returned error")
		return studentRegs, err
	}

	return studentRegs, nil
}
func (db *DB) ChangeAncodeInStudentRegs(ctx context.Context, program string, anCode, newAncode int) ([]*model.StudentReg, error) {
	err := db.ChangeAncodeInStudentRegsCount(ctx, program, anCode, newAncode)
	if err != nil {
		return nil, err
	}
	collection := db.getCollection(program, StudentRegs)

	filter := bson.D{{Key: "AnCode", Value: anCode}}
	update := bson.D{{Key: "$set", Value: bson.D{{Key: "AnCode", Value: newAncode}}}}

	result, err := collection.UpdateMany(ctx, filter, update)

	if err != nil {
		log.Error().Err(err).
			Str("program", program).Int("from", anCode).Int("to", newAncode).
			Msg("error while trying to change ancode.")
		return nil, err
	}

	if result.MatchedCount == 0 {
		log.Debug().
			Str("program", program).Int("from", anCode).Int("to", newAncode).
			Msg("no student regs updated while trying to change ancode.")
	}

	return db.GetPrimussStudentRegsForProgrammAncode(ctx, program, newAncode)
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

func (db *DB) ChangeAncodeInStudentRegsCount(ctx context.Context, program string, anCode, newAncode int) error {
	collection := db.getCollection(program, Counts)

	filter := bson.D{{Key: "AnCo", Value: anCode}}
	update := bson.D{{Key: "$set", Value: bson.D{{Key: "AnCo", Value: newAncode}}}}

	result, err := collection.UpdateMany(ctx, filter, update)

	if err != nil {
		log.Error().Err(err).
			Str("program", program).Int("from", anCode).Int("to", newAncode).
			Msg("error while trying to change ancode in count.")
		return err
	}

	if result.MatchedCount == 0 {
		log.Debug().
			Str("program", program).Int("from", anCode).Int("to", newAncode).
			Msg("no count of student regs updated while trying to change ancode.")
	}

	return nil
}

func (db *DB) SetRegsWithErrors(ctx context.Context, regsWithErrors []*model.RegWithError) error {
	collectionName := "errors-zpa-studentregs"
	collection := db.Client.Database(databaseName(db.semester)).Collection(collectionName)

	err := collection.Drop(ctx)
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("collection", collectionName).Msg("cannot drop collection")
		return err
	}

	regsWithErrorsIntf := make([]interface{}, 0, len(regsWithErrors))

	for _, v := range regsWithErrors {
		regsWithErrorsIntf = append(regsWithErrorsIntf, v)
	}

	_, err = collection.InsertMany(ctx, regsWithErrorsIntf)
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("collection", collectionName).Msg("cannot insert documents")
		return err
	}

	return nil
}

func (db *DB) GetRegsWithErrors(ctx context.Context) ([]*model.RegWithError, error) {
	collectionName := "errors-zpa-studentregs"
	collection := db.Client.Database(databaseName(db.semester)).Collection(collectionName)

	regWithErrors := make([]*model.RegWithError, 0)

	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Msg("MongoDB Find (reg with errors)")
		return regWithErrors, err
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var regWithError model.RegWithError

		err := cur.Decode(&regWithError)
		if err != nil {
			log.Error().Err(err).Str("semester", db.semester).Interface("cur", cur).Msg("Cannot decode to regWithError")
			return regWithErrors, err
		}

		regWithErrors = append(regWithErrors, &regWithError)
	}

	if err := cur.Err(); err != nil {
		log.Error().Err(err).Str("semester", db.semester).Msg("Cursor returned error")
		return regWithErrors, err
	}

	return regWithErrors, nil
}
