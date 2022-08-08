package db

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type PrimussType string

const (
	StudentRegs PrimussType = "studentregs"
	Exams       PrimussType = "exams"
	Counts      PrimussType = "count"
	Conflicts   PrimussType = "conflicts"
)

func (db *DB) getCollection(program string, primussType PrimussType) *mongo.Collection {
	return db.Client.Database(databaseName(db.semester)).Collection(fmt.Sprintf("%s_%s", primussType, program))
}

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
	parts := strings.Split(collectionName, "_")
	collection := db.getCollection(parts[1], Exams)

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

func (db *DB) GetPrimussConflictsForAncode(ctx context.Context, program string, anCode int) (*model.Conflicts, error) {
	conflicts, err := db.getConflictsForAnCode(ctx, program, anCode)
	if err != nil {
		return nil, err
	}

	conflictsSlice := make([]model.Conflict, 0)
	for k, v := range conflicts.Conflicts {
		conflictsSlice = append(conflictsSlice, model.Conflict{
			AnCode:        k,
			NumberOfStuds: v,
		})
	}

	return &model.Conflicts{
		AnCode:     conflicts.AnCode,
		Module:     conflicts.Module,
		MainExamer: conflicts.MainExamer,
		Conflicts:  conflictsSlice,
	}, nil
}

type Conflict struct {
	AnCode     int
	Module     string
	MainExamer string
	Conflicts  map[int]int
}

func (db *DB) getConflictsForAnCode(ctx context.Context, program string, anCode int) (*Conflict, error) {
	collection := db.getCollection(program, Conflicts)
	raw, err := collection.FindOne(ctx, bson.D{{Key: "AnCo", Value: anCode}}).DecodeBytes()
	if err != nil {
		log.Error().Err(err).Str("program", program).Int("anCode", anCode).Msg("cannot get conflicts for anCode")
		return nil, err
	}

	conflict, err := decode(&raw)
	if err != nil {
		log.Error().Err(err).Str("program", program).Int("anCode", anCode).Msg("cannot decode raw to conflict")
		return nil, err
	}
	return conflict, nil
}

func decode(raw *bson.Raw) (*Conflict, error) {
	elements, err := raw.Elements()
	if err != nil {
		return nil, err
	}

	conflict := &Conflict{
		AnCode:     0,
		Module:     "",
		MainExamer: "",
		Conflicts:  make(map[int]int),
	}

	for _, elem := range elements {
		switch elem.Key() {
		case "AnCo":
			conflict.AnCode = int(elem.Value().Int32())
		case "Titel":
			conflict.Module = elem.Value().StringValue()
		case "Prüfer":
			conflict.MainExamer = elem.Value().StringValue()
		case "_id":
			continue
		default:
			anCode, err := strconv.ParseInt(elem.Key(), 10, 32)
			if err != nil {
				log.Debug().Str("anCode?", elem.Key()).Msg("cannot convert key to ancode")
			}
			conflict.Conflicts[int(anCode)] = int(elem.Value().Int32())
		}
	}

	return conflict, nil
}
