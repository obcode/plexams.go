package db

import (
	"context"
	"sort"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

const (
	collectionStudentRegsPerAncodePlanned  = "studentregs_per_ancode_planned"
	collectionStudentRegsPerAncodeAll      = "studentregs_per_ancode_all"
	collectionStudentRegsPerStudentPlanned = "studentregs_per_student_planned"
	collectionStudentRegsPerStudentAll     = "studentregs_per_student_all"
)

func (db *DB) SaveStudentRegsPerAncode(ctx context.Context, studentRegsPerAncode map[int]map[string][]*model.StudentReg, all bool) error {
	ancodes := make([]int, 0, len(studentRegsPerAncode))
	for ancode := range studentRegsPerAncode {
		ancodes = append(ancodes, ancode)
	}
	sort.Ints(ancodes)

	studentRegsPerAncodeSlice := make([]interface{}, 0)

	for _, ancode := range ancodes {
		studentRegsWithProgram := studentRegsPerAncode[ancode]
		programs := make([]string, 0, len(studentRegsWithProgram))
		for program, _ := range studentRegsWithProgram {
			programs = append(programs, program)
		}
		sort.Strings(programs)

		perProgram := make([]*model.StudentRegsPerAncodeAndProgram, 0)
		for _, program := range programs {
			perProgram = append(perProgram, &model.StudentRegsPerAncodeAndProgram{
				Program:     program,
				StudentRegs: studentRegsWithProgram[program],
			})
		}

		studentRegsPerAncodeSlice = append(studentRegsPerAncodeSlice, &model.StudentRegsPerAncode{
			AnCode:     ancode,
			PerProgram: perProgram,
		})
	}

	collectionName := collectionStudentRegsPerAncodePlanned
	if all {
		collectionName = collectionStudentRegsPerAncodeAll
	}

	collection := db.Client.Database(databaseName(db.semester)).Collection(collectionName)

	err := collection.Drop(ctx)
	if err != nil {
		log.Error().Err(err).Str("collectionName", collectionName).
			Msg("error while trying to drop the collection")
		return err
	}

	_, err = collection.InsertMany(ctx, studentRegsPerAncodeSlice)
	if err != nil {
		log.Error().Err(err).Str("collectionName", collectionName).
			Msg("error while trying to insert")
		return err
	}

	return nil
}

func (db *DB) SaveStudentRegsPerStudent(ctx context.Context, studentRegsPerStudent map[string][]*model.StudentReg, all bool) error {
	studentRegsSlice := make([]interface{}, 0)

	for mtknr, regs := range studentRegsPerStudent {
		if len(regs) > 0 {
			ancodes := make([]int, 0, len(regs))
			for _, reg := range regs {
				ancodes = append(ancodes, reg.AnCode)
			}

			studentRegsSlice = append(studentRegsSlice, &model.StudentRegsPerStudent{
				Student: &model.Student{
					Mtknr:   mtknr,
					Program: regs[0].Program,
					Group:   regs[0].Group,
					Name:    regs[0].Name,
				},
				AnCodes: ancodes,
			})
		}
	}

	collectionName := collectionStudentRegsPerStudentPlanned
	if all {
		collectionName = collectionStudentRegsPerStudentAll
	}

	collection := db.Client.Database(databaseName(db.semester)).Collection(collectionName)

	err := collection.Drop(ctx)
	if err != nil {
		log.Error().Err(err).Str("collectionName", collectionName).
			Msg("error while trying to drop the collection")
		return err
	}

	_, err = collection.InsertMany(ctx, studentRegsSlice)
	if err != nil {
		log.Error().Err(err).Str("collectionName", collectionName).
			Msg("error while trying to insert")
		return err
	}

	return nil
}
