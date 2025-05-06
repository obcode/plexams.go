package db

import (
	"context"
	"sort"

	set "github.com/deckarep/golang-set/v2"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (db *DB) GetStudentRegsPerAncodePlanned(ctx context.Context) ([]*model.StudentRegsPerAncode, error) {
	collection := db.Client.Database(db.databaseName).Collection(collectionStudentRegsPerAncodePlanned)

	studentRegs := make([]*model.StudentRegsPerAncode, 0)

	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Msg("MongoDB Find")
		return nil, err
	}
	defer cur.Close(ctx) //nolint:errcheck

	for cur.Next(ctx) {
		var studentReg model.StudentRegsPerAncode

		err := cur.Decode(&studentReg)
		if err != nil {
			log.Error().Err(err).Str("semester", db.semester).Interface("cur", cur).
				Msg("Cannot decode to studentReg")
			return nil, err
		}

		studentRegs = append(studentRegs, &studentReg)
	}

	if err := cur.Err(); err != nil {
		log.Error().Err(err).Str("semester", db.semester).Msg("Cursor returned error")
		return nil, err
	}

	return studentRegs, nil
}

func (db *DB) StudentRegsPerStudentPlanned(ctx context.Context) ([]*model.Student, error) {
	collection := db.Client.Database(db.databaseName).Collection(collectionStudentRegsPerStudentPlanned)

	findOptions := options.Find()
	findOptions.SetSort(bson.D{{Key: "name", Value: 1}})

	cur, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Msg("MongoDB Find")
		return nil, err
	}
	defer cur.Close(ctx) //nolint:errcheck

	studentRegs := make([]*model.Student, 0)

	err = cur.All(ctx, &studentRegs)
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Interface("cur", cur).
			Msg("Cannot decode to studentRegs")
		return nil, err
	}

	return studentRegs, nil
}

func (db *DB) StudentRegsPerStudentAll(ctx context.Context) ([]*model.StudentRegsPerStudent, error) {
	collection := db.Client.Database(db.databaseName).Collection(collectionStudentRegsPerStudentAll)

	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Msg("MongoDB Find")
		return nil, err
	}
	defer cur.Close(ctx) //nolint:errcheck

	studentRegs := make([]*model.StudentRegsPerStudent, 0)

	err = cur.All(ctx, &studentRegs)
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Interface("cur", cur).
			Msg("Cannot decode to studentRegs")
		return nil, err
	}

	return studentRegs, nil
}

func (db *DB) StudentByMtknr(ctx context.Context, mtknr string, ntas map[string]*model.NTA) (*model.Student, error) {
	collectionNames, err := db.studentRegsCollectionNames(ctx)

	if err != nil {
		log.Error().Err(err).Msg("cannot get student regs collections")
		return nil, err
	}

	var student *model.Student

	for _, collectionName := range collectionNames {
		log.Debug().Str("collection", collectionName).Str("mtkntr", mtknr).
			Msg("searching for student in collection")

		collection := db.Client.Database(db.databaseName).Collection(collectionName)

		cur, err := collection.Find(ctx, bson.D{{Key: "MTKNR", Value: mtknr}})
		if err != nil {
			log.Error().Err(err).Str("collection", collectionName).Str("mtkntr", mtknr).
				Msg("error while searching for student in collection")
		}
		defer cur.Close(ctx) //nolint:errcheck

		var results []*model.StudentReg

		err = cur.All(ctx, &results)
		if err != nil {
			log.Error().Err(err).Str("collection", collectionName).Str("mtkntr", mtknr).
				Msg("error while decoding student from collection")
		}

		if len(results) > 0 {
			log.Debug().Interface("regs", results).Str("collection", collectionName).Str("mtkntr", mtknr).
				Msg("found regs for student")

			var regs []int

			if student != nil && (student.Program != results[0].Program ||
				student.Group != results[0].Group ||
				student.Name != results[0].Name) {
				log.Error().Str("collection", collectionName).Str("mtkntr", mtknr).
					Msg("found student in more than one programs")
			}

			if student != nil {
				regs = student.Regs
			} else {
				regs = make([]int, 0, len(results))
			}

			for _, res := range results {
				regs = append(regs, res.AnCode)
			}

			sort.Ints(regs)

			var nta *model.NTA

			if ntas == nil {
				nta, err = db.Nta(ctx, mtknr)
				if err != nil {
					log.Error().Err(err).Str("mtknr", mtknr).Msg("error while checking nta")
				}
			} else {
				nta = ntas[mtknr]
			}

			student = &model.Student{
				Mtknr:   mtknr,
				Program: results[0].Program,
				Group:   results[0].Group,
				Name:    results[0].Name,
				Regs:    regs,
				Nta:     nta,
			}

		}

	}

	return student, nil
}

func (db *DB) StudentsByName(ctx context.Context, regex string) ([]*model.Student, error) {
	collectionNames, err := db.studentRegsCollectionNames(ctx)

	if err != nil {
		log.Error().Err(err).Msg("cannot get student regs collections")
		return nil, err
	}

	studentMtknrs := set.NewSet[string]()

	for _, collectionName := range collectionNames {
		collection := db.Client.Database(db.databaseName).Collection(collectionName)

		cur, err := collection.Find(ctx, bson.D{{Key: "name", Value: bson.D{
			primitive.E{Key: "$regex",
				Value: primitive.Regex{Pattern: regex},
			}}}})
		if err != nil {
			log.Error().Err(err).Str("collection", collectionName).Str("regex", regex).
				Msg("error while searching for student in collection")
		}
		defer cur.Close(ctx) //nolint:errcheck

		for cur.Next(ctx) {
			mtknr := cur.Current.Lookup("MTKNR").StringValue()
			studentMtknrs.Add(mtknr)
		}
	}

	students := make([]*model.Student, 0, studentMtknrs.Cardinality())

	for _, mtknr := range studentMtknrs.ToSlice() {
		student, err := db.StudentByMtknr(ctx, mtknr, nil)
		if err != nil {
			log.Error().Err(err).Str("mtknr", mtknr).Msg("error while trying to get student")
		} else {
			students = append(students, student)
		}
	}

	return students, nil
}

func (db *DB) studentRegsCollectionNames(ctx context.Context) ([]string, error) {
	return db.Client.Database(db.databaseName).ListCollectionNames(ctx,
		bson.D{primitive.E{
			Key: "name",
			Value: bson.D{
				primitive.E{Key: "$regex",
					Value: primitive.Regex{Pattern: "studentregs_..$"},
				},
			},
		}})
}
