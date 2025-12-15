package db

import (
	"context"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
)

func (db *DB) GetZPAStudents(ctx context.Context) ([]*model.ZPAStudent, error) {
	collection := db.getCollectionSemester(collectionZpaStudents)

	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Msg("cannot get zpa students")
		return nil, err
	}
	defer cur.Close(ctx) //nolint:errcheck

	zpaStudents := make([]*model.ZPAStudent, 0)

	err = cur.All(ctx, &zpaStudents)
	if err != nil {
		log.Error().Err(err).Msg("cannot decode zpa students")
		return nil, err
	}

	return zpaStudents, nil
}

func (db *DB) GetZPAStudentByMtknr(ctx context.Context, mtknr string) (*model.ZPAStudent, error) {
	var zpaStudent model.ZPAStudent

	collection := db.getCollectionSemester(collectionZpaStudents)

	err := collection.FindOne(ctx, bson.D{{Key: "mtknr", Value: mtknr}}).Decode(&zpaStudent)
	if err != nil {
		log.Debug().Err(err).Str("mtknr", mtknr).Msg("cannot find zpa student in db")
		return nil, err
	}

	return &zpaStudent, nil
}

func (db *DB) GetTeacher(ctx context.Context, id int) (*model.Teacher, error) {
	var teacher model.Teacher

	if id == 0 {
		return &teacher, nil
	}

	collection := db.Client.Database(db.databaseName).Collection("teachers")

	err := collection.FindOne(ctx, bson.D{{Key: "id", Value: id}}).Decode(&teacher)
	if err != nil {
		log.Error().Err(err).Int("id", id).Msg("cannot find teacher in db")
		return nil, err
	}

	return &teacher, nil
}

func (db *DB) GetTeacherIdByRegex(ctx context.Context, name string) (int, error) {
	collection := db.Client.Database(db.databaseName).Collection("teachers")

	var teacher model.Teacher
	err := collection.FindOne(ctx, bson.D{{Key: "fullname", Value: bson.D{{Key: "$regex", Value: name}}}}).Decode(&teacher)
	if err != nil {
		log.Error().Err(err).Str("name", name).Msg("cannot find teacher in db")
		return 0, err
	}

	return teacher.ID, nil
}

func (db *DB) GetTeachers(ctx context.Context) ([]*model.Teacher, error) {
	return db.getTeachers(ctx, func(model.Teacher) bool { return true })
}

func (db *DB) GetInvigilators(ctx context.Context) ([]*model.Teacher, error) {
	return db.getTeachers(ctx, func(teacher model.Teacher) bool {
		return isInvigilator(teacher)
		// return isInvigilator(teacher, db.semester)
	})
}

func isInvigilator(teacher model.Teacher) bool {
	// func isInvigilator(teacher model.Teacher, semester string) bool {
	return teacher.IsProf &&
		!teacher.IsProfHC &&
		!teacher.IsLBA &&
		teacher.FK == "FK07" // &&
	// strings.Compare(semester, teacher.LastSemester) <= 0
}

func (db *DB) getTeachers(ctx context.Context, predicate func(model.Teacher) bool) ([]*model.Teacher, error) {
	collection := db.Client.Database(db.databaseName).Collection("teachers")

	teachers := make([]*model.Teacher, 0)

	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("collection", "teachers").Msg("MongoDB Find")
		return teachers, err
	}
	defer cur.Close(ctx) //nolint:errcheck

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

func (db *DB) CacheTeachers(teachers []*model.Teacher, semester string) error {
	collection := db.Client.Database(db.databaseName).Collection("teachers")

	teachersIntf := make([]interface{}, 0, len(teachers))

	for _, v := range teachers {
		teachersIntf = append(teachersIntf, v)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := collection.Drop(ctx)
	if err != nil {
		return err
	}

	res, err := collection.InsertMany(ctx, teachersIntf)
	if err != nil {
		return err
	}

	log.Debug().Str("semester", semester).Int("documents", len(res.InsertedIDs)).Msg("inserted teachers")

	return nil
}
