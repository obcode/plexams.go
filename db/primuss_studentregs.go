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

func (db *DB) GetPrimussStudentRegsForProgrammAncode(ctx context.Context, program string, ancode int) ([]*model.StudentReg, error) {
	collection := db.getCollection(program, StudentRegs)

	findOptions := options.Find()
	findOptions.SetSort(bson.D{{Key: "name", Value: 1}})

	cur, err := collection.Find(ctx, bson.D{{Key: "AnCode", Value: ancode}}, findOptions)
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Str("program", program).Msg("MongoDB Find (studentregs)")
		return nil, err
	}
	defer cur.Close(ctx) //nolint:errcheck

	var studentRegs []*model.StudentReg

	err = cur.All(ctx, &studentRegs)
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Str("program", program).Msg("cannot decode to studentregs")
		return nil, err
	}

	return studentRegs, nil
}

func (db *DB) GetPrimussStudentRegsPerAncode(ctx context.Context, program string) (map[int][]*model.StudentReg, error) {
	collection := db.getCollection(program, StudentRegs)

	studentRegs := make(map[int][]*model.StudentReg)

	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("program", program).Msg("MongoDB Find (studentregs)")
		return studentRegs, err
	}
	defer cur.Close(ctx) //nolint:errcheck

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
		if !db.CheckStudentRegsCount(ctx, program, k, len(v)) {
			return nil, fmt.Errorf("problem with studentregs, ancode = %d, #studentregs = %d", k, len(v))
		}
	}

	if err := cur.Err(); err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("program", program).Msg("Cursor returned error")
		return studentRegs, err
	}

	return studentRegs, nil
}

func (db *DB) GetPrimussStudentRegsPerStudent(ctx context.Context, program string) (map[string][]*model.StudentReg, error) {
	collection := db.getCollection(program, StudentRegs)

	studentRegs := make(map[string][]*model.StudentReg)

	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("program", program).Msg("MongoDB Find (studentregs)")
		return studentRegs, err
	}
	defer cur.Close(ctx) //nolint:errcheck

	for cur.Next(ctx) {
		var studentReg model.StudentReg

		err := cur.Decode(&studentReg)
		if err != nil {
			log.Error().Err(err).Str("semester", db.semester).Str("program", program).Interface("cur", cur).
				Msg("Cannot decode to exam")
			return studentRegs, err
		}

		regs, ok := studentRegs[studentReg.Mtknr]
		if !ok {
			regs = make([]*model.StudentReg, 0)
		}

		studentRegs[studentReg.Mtknr] = append(regs, &studentReg)

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
	defer cur.Close(ctx) //nolint:errcheck

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
func (db *DB) ChangeAncodeInStudentRegs(ctx context.Context, program string, ancode, newAncode int) ([]*model.StudentReg, error) {
	err := db.ChangeAncodeInStudentRegsCount(ctx, program, ancode, newAncode)
	if err != nil {
		return nil, err
	}
	collection := db.getCollection(program, StudentRegs)

	filter := bson.D{{Key: "AnCode", Value: ancode}}
	update := bson.D{{Key: "$set", Value: bson.D{{Key: "AnCode", Value: newAncode}}}}

	result, err := collection.UpdateMany(ctx, filter, update)

	if err != nil {
		log.Error().Err(err).
			Str("program", program).Int("from", ancode).Int("to", newAncode).
			Msg("error while trying to change ancode.")
		return nil, err
	}

	if result.MatchedCount == 0 {
		log.Debug().
			Str("program", program).Int("from", ancode).Int("to", newAncode).
			Msg("no student regs updated while trying to change ancode.")
	}

	return db.GetPrimussStudentRegsForProgrammAncode(ctx, program, newAncode)
}

type Count struct {
	AnCode int `bson:"AnCo"`
	Sum    int `bson:"Sum"`
}

func (db *DB) CheckStudentRegsCount(ctx context.Context, program string, ancode, studentRegsCount int) bool {
	// log.Debug().Str("collectionName", collectionName).Int("ancode", ancode).Int("studentRegsCount", studentRegsCount).
	// 	Msg("checking count")
	collection := db.getCollection(program, Counts)
	var result Count
	err := collection.FindOne(ctx, bson.D{{Key: "AnCo", Value: ancode}}).Decode(&result)
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("program", program).
			Int("ancode", ancode).Int("studentRegsCount", studentRegsCount).Msg("error finding count")
		return false
	}
	if result.Sum != studentRegsCount {
		log.Debug().Str("semester", db.semester).Str("program", program).
			Int("ancode", ancode).Int("studentRegsCount", studentRegsCount).Int("result.Sum", result.Sum).
			Msg("sum != student registrations")
		return false
	}
	return true
}

func (db *DB) GetStudentRegsCount(ctx context.Context, program string, ancode int) (int, error) {
	// log.Debug().Str("collectionName", collectionName).Int("ancode", ancode).Int("studentRegsCount", studentRegsCount).
	// 	Msg("checking count")
	collection := db.getCollection(program, Counts)
	var result Count
	res := collection.FindOne(ctx, bson.D{{Key: "AnCo", Value: ancode}, {Key: "Sum", Value: bson.D{{Key: "$ne", Value: ""}}}})
	if res.Err() == mongo.ErrNoDocuments {
		return 0, nil
	}
	err := res.Decode(&result)
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("program", program).
			Int("ancode", ancode).Msg("error finding count")
		return -1, err
	}

	return result.Sum, nil
}

func (db *DB) ChangeAncodeInStudentRegsCount(ctx context.Context, program string, ancode, newAncode int) error {
	collection := db.getCollection(program, Counts)

	filter := bson.D{{Key: "AnCo", Value: ancode}}
	update := bson.D{{Key: "$set", Value: bson.D{{Key: "AnCo", Value: newAncode}}}}

	result, err := collection.UpdateMany(ctx, filter, update)

	if err != nil {
		log.Error().Err(err).
			Str("program", program).Int("from", ancode).Int("to", newAncode).
			Msg("error while trying to change ancode in count.")
		return err
	}

	if result.MatchedCount == 0 {
		log.Debug().
			Str("program", program).Int("from", ancode).Int("to", newAncode).
			Msg("no count of student regs updated while trying to change ancode.")
	}

	return nil
}

func (db *DB) SetRegsWithErrors(ctx context.Context, regsWithErrors []*model.RegWithError) error {
	collectionName := "errors-zpa-studentregs"
	collection := db.Client.Database(db.databaseName).Collection(collectionName)

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
	collection := db.Client.Database(db.databaseName).Collection(collectionName)

	regWithErrors := make([]*model.RegWithError, 0)

	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Msg("MongoDB Find (reg with errors)")
		return regWithErrors, err
	}
	defer cur.Close(ctx) //nolint:errcheck

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

func (db *DB) RemoveStudentReg(ctx context.Context, program string, ancode int, mtknr string) (int, error) {
	collection := db.getCollection(program, StudentRegs)

	filter := bson.M{
		"$and": []bson.M{
			{"AnCode": ancode},
			{"MTKNR": mtknr},
		},
	}

	res, err := collection.DeleteOne(ctx, filter)
	if err != nil {
		log.Error().Err(err).Str("program", program).Int("ancode", ancode).Str("mtknr", mtknr).
			Msg("error while trying to delete")
		return int(res.DeletedCount), err
	}

	collection = db.getCollection(program, Counts)

	filterUpdate := bson.D{{Key: "AnCo", Value: ancode}}
	update := bson.D{{Key: "$inc", Value: bson.D{{Key: "Sum", Value: -1}}}}

	result, err := collection.UpdateOne(ctx, filterUpdate, update)

	if err != nil {
		log.Error().Err(err).
			Str("program", program).Int("ancode", ancode).
			Msg("error while trying to change sum in count.")
		return int(result.MatchedCount), err
	}

	if result.MatchedCount == 0 {
		log.Debug().
			Str("program", program).Int("from", ancode).
			Msg("no count of student regs updated while trying to change sum.")
	}

	return int(res.DeletedCount), nil
}

func (db *DB) AddStudentReg(ctx context.Context, program string, ancode int, mtknr string) error {
	collection := db.getCollection(program, StudentRegs)

	student, err := db.StudentByMtknr(ctx, mtknr, nil)
	if err != nil {
		log.Error().Err(err).Str("program", program).Int("ancode", ancode).Str("mtknr", mtknr).
			Msg("error while trying to get student by mtknr")
		return err
	}

	doc := bson.D{
		{Key: "AnCode", Value: ancode},
		{Key: "MTKNR", Value: mtknr},
		{Key: "name", Value: student.Name},
	}

	_, err = collection.InsertOne(ctx, doc)
	if err != nil {
		log.Error().Err(err).Str("program", program).Int("ancode", ancode).Str("mtknr", mtknr).
			Msg("error while trying to insert")
		return err
	}

	collection = db.getCollection(program, Counts)

	filterUpdate := bson.D{{Key: "AnCo", Value: ancode}}
	update := bson.D{{Key: "$inc", Value: bson.D{{Key: "Sum", Value: 1}}}}

	_, err = collection.UpdateOne(ctx, filterUpdate, update)

	if err != nil {
		log.Error().Err(err).
			Str("program", program).Int("ancode", ancode).
			Msg("error while trying to change sum in count.")
		return err
	}

	return nil
}
