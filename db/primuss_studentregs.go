package db

import (
	"context"
	"sort"

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

		regs, ok := studentRegs[studentReg.PrimussAncode]
		if !ok {
			regs = make([]*model.StudentReg, 0)
		}

		studentRegs[studentReg.PrimussAncode] = append(regs, &studentReg)

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

// StudentRegsCountMismatch is one exam whose recorded sum in count_<program>
// disagrees with the registrations actually stored in studentregs_<program>.
type StudentRegsCountMismatch struct {
	Program string
	Ancode  int
	// Stored is the number of registration documents; Recorded is the Sum in the
	// count collection, or NoCountDocument when the exam has no count document.
	Stored   int
	Recorded int
}

// NoCountDocument marks a mismatch where the count document is missing entirely.
const NoCountDocument = -1

// StudentRegsCountMismatches compares the stored registrations of a program against
// the counts Primuss delivered alongside them. The two drift apart when a single
// registration is added or removed, because that writes both collections without a
// transaction (AddStudentReg / RemoveStudentReg).
//
// Reported, never enforced: this used to abort GetPrimussStudentRegsPerAncode with an
// error, which took the whole exam generation down until someone repaired the counter
// by hand. A drift is a data-quality finding for the validation report.
func (db *DB) StudentRegsCountMismatches(ctx context.Context, program string) ([]StudentRegsCountMismatch, error) {
	stored := make(map[int]int)
	cur, err := db.getCollection(program, StudentRegs).Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	var regs []model.StudentReg
	if err := cur.All(ctx, &regs); err != nil {
		return nil, err
	}
	for _, reg := range regs {
		stored[reg.PrimussAncode]++
	}

	recorded := make(map[int]int)
	cur, err = db.getCollection(program, Counts).Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	var counts []Count
	if err := cur.All(ctx, &counts); err != nil {
		return nil, err
	}
	for _, c := range counts {
		recorded[c.AnCode] = c.Sum
	}

	mismatches := make([]StudentRegsCountMismatch, 0)
	for ancode, n := range stored {
		sum, ok := recorded[ancode]
		if !ok {
			mismatches = append(mismatches, StudentRegsCountMismatch{
				Program: program, Ancode: ancode, Stored: n, Recorded: NoCountDocument,
			})
			continue
		}
		if sum != n {
			mismatches = append(mismatches, StudentRegsCountMismatch{
				Program: program, Ancode: ancode, Stored: n, Recorded: sum,
			})
		}
	}
	sort.Slice(mismatches, func(i, j int) bool { return mismatches[i].Ancode < mismatches[j].Ancode })
	return mismatches, nil
}

func (db *DB) GetStudentRegsCount(ctx context.Context, program string, ancode int) (int, error) {
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

// RemoveStudentReg deletes one registration and decrements the Primuss counter. Both
// writes run in one transaction where the deployment allows it, so the counter cannot
// drift when the second one fails (see StudentRegsCountMismatches for the symptom).
func (db *DB) RemoveStudentReg(ctx context.Context, program string, ancode int, mtknr string) (int, error) {
	var deleted int
	err := db.withTransaction(ctx, func(ctx context.Context) error {
		var err error
		deleted, err = db.removeStudentReg(ctx, program, ancode, mtknr)
		return err
	})
	return deleted, err
}

func (db *DB) removeStudentReg(ctx context.Context, program string, ancode int, mtknr string) (int, error) {
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

// AddStudentReg inserts one registration and increments the Primuss counter. Both
// writes run in one transaction where the deployment allows it, so the counter cannot
// drift when the second one fails (see StudentRegsCountMismatches for the symptom).
func (db *DB) AddStudentReg(ctx context.Context, program string, ancode int, mtknr string) error {
	return db.withTransaction(ctx, func(ctx context.Context) error {
		return db.addStudentReg(ctx, program, ancode, mtknr)
	})
}

func (db *DB) addStudentReg(ctx context.Context, program string, ancode int, mtknr string) error {
	collection := db.getCollection(program, StudentRegs)

	student, err := db.StudentByMtknr(ctx, mtknr)
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
