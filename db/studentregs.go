package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
)

func (db *DB) GetStudentRegsPerAncodePlanned(ctx context.Context) ([]*model.StudentRegsPerAncode, error) {
	collection := db.Client.Database(databaseName(db.semester)).Collection(collectionStudentRegsPerAncodePlanned)

	studentRegs := make([]*model.StudentRegsPerAncode, 0)

	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Msg("MongoDB Find")
		return nil, err
	}
	defer cur.Close(ctx)

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

func (db *DB) StudentRegsPerStudentPlanned(ctx context.Context) ([]*model.StudentRegsPerStudent, error) {
	collection := db.Client.Database(databaseName(db.semester)).Collection(collectionStudentRegsPerStudentPlanned)

	studentRegs := make([]*model.StudentRegsPerStudent, 0)

	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Msg("MongoDB Find")
		return nil, err
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var studentReg model.StudentRegsPerStudent

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
