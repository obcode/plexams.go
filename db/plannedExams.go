package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
)

// Deprecated: rm me
func (db *DB) ExamsInSlot(ctx context.Context, day int, time int) ([]*model.ExamInPlan, error) {
	collection := db.getCollectionSemester(collectionExamsInPlan)

	filter := bson.M{
		"$and": []bson.M{
			{"slot.daynumber": day},
			{"slot.slotnumber": time},
		},
	}
	cur, err := collection.Find(ctx, filter)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionNamePlan).Msg("MongoDB Find")
		return nil, err
	}
	defer cur.Close(ctx)

	exams := make([]*model.ExamInPlan, 0)
	err = cur.All(ctx, &exams)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionNameNTAs).Msg("Cannot decode to exams")
		return nil, err
	}

	return exams, nil
}

// Deprecated: rm me
func (db *DB) PlannedExamsByMainExamer(ctx context.Context, examerID int) ([]*model.ExamInPlan, error) {
	collection := db.getCollectionSemester(collectionExamsInPlan)

	filter := bson.D{{Key: "exam.zpaexam.mainexamerid", Value: examerID}}

	cur, err := collection.Find(ctx, filter)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionNamePlan).Msg("MongoDB Find")
		return nil, err
	}
	defer cur.Close(ctx)

	exams := make([]*model.ExamInPlan, 0)
	err = cur.All(ctx, &exams)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionNameNTAs).Msg("Cannot decode to exams")
		return nil, err
	}

	return exams, nil
}

// func (db *DB)
