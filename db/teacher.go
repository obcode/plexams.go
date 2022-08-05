package db

import (
	"context"
	"strings"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
)

func (db *DB) GetTeachers(ctx context.Context) ([]*model.Teacher, error) {
	return db.getTeachers(ctx, func(model.Teacher) bool { return true })
}

func (db *DB) GetInvigilators(ctx context.Context) ([]*model.Teacher, error) {
	return db.getTeachers(ctx, func(teacher model.Teacher) bool {
		return isInvigilator(teacher, db.semester)
	})
}

func isInvigilator(teacher model.Teacher, semester string) bool {
	return teacher.IsProf &&
		!teacher.IsProfHC &&
		teacher.FK == "FK07" &&
		strings.Compare(semester, teacher.LastSemester) <= 0
}

func (db *DB) getTeachers(ctx context.Context, predicate func(model.Teacher) bool) ([]*model.Teacher, error) {
	collection := db.Client.Database(databaseName(db.semester)).Collection("teachers")

	teachers := make([]*model.Teacher, 0)

	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("collection", "teachers").Msg("MongoDB Find")
		return teachers, err
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var teacher model.Teacher

		err := cur.Decode(&teacher)
		if err != nil {
			log.Error().Err(err).Str("semester", db.semester).Str("collection", "teachers").Interface("cur", cur).
				Msg("Cannot decode to customer")
			return teachers, err
		}

		if predicate(teacher) {
			teachers = append(teachers, &teacher)
		}
	}

	if err := cur.Err(); err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("collection", "teachers").Msg("Cursor returned error")
		return teachers, err
	}

	return teachers, nil
}
