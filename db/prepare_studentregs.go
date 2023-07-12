package db

import (
	"context"
	"sort"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
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
		for program := range studentRegsWithProgram {
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
			Ancode:     ancode,
			PerProgram: perProgram,
		})
	}

	collectionName := collectionStudentRegsPerAncodePlanned
	if all {
		collectionName = collectionStudentRegsPerAncodeAll
	}

	collection := db.Client.Database(db.databaseName).Collection(collectionName)

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
				Ancodes: ancodes,
			})
		}
	}

	collectionName := collectionStudentRegsPerStudentPlanned
	if all {
		collectionName = collectionStudentRegsPerStudentAll
	}

	collection := db.Client.Database(db.databaseName).Collection(collectionName)

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
