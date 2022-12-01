package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const collectionNameExamGroups = "exam_groups"

func (db *DB) SaveExamGroups(ctx context.Context, exams []*model.ExamGroup) error {
	collection := db.Client.Database(databaseName(db.semester)).Collection(collectionNameExamGroups)

	err := collection.Drop(ctx)
	if err != nil {
		log.Error().Err(err).
			Str("collectionName", collectionNameExamGroups).
			Msg("cannot drop collection")
		return err
	}

	examGroupsToInsert := make([]interface{}, 0, len(exams))
	for _, exam := range exams {
		examGroupsToInsert = append(examGroupsToInsert, exam)
	}

	_, err = collection.InsertMany(ctx, examGroupsToInsert)
	if err != nil {
		log.Error().Err(err).
			Str("collectionName", collectionNameExamGroups).
			Msg("cannot insert exams")
		return err
	}

	return nil
}

func (db *DB) ExamGroup(ctx context.Context, examGroupCode int) (*model.ExamGroup, error) {
	collection := db.Client.Database(databaseName(db.semester)).Collection(collectionNameExamGroups)

	res := collection.FindOne(ctx, bson.D{{Key: "examgroupcode", Value: examGroupCode}})
	if res.Err() != nil {
		log.Error().Err(res.Err()).Int("examGroupCode", examGroupCode).Msg("no exam group found")
		return nil, res.Err()
	}
	var examGroup model.ExamGroup
	err := res.Decode(&examGroup)
	if err != nil {
		log.Error().Err(res.Err()).Int("examGroupCode", examGroupCode).Msg("cannot decode constraint")
		return nil, err
	}

	return &examGroup, nil
}

func (db *DB) GetExamGroupForAncode(ctx context.Context, ancode int) (*model.ExamGroup, error) {
	examGroups, err := db.ExamGroups(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get exam groups")
		return nil, err
	}

	for _, group := range examGroups {
		for _, exam := range group.Exams {
			if exam.Exam.Ancode == ancode {
				return group, nil
			}
		}
	}

	return nil, nil
}

func (db *DB) ExamGroups(ctx context.Context) ([]*model.ExamGroup, error) {
	collection := db.Client.Database(databaseName(db.semester)).Collection(collectionNameExamGroups)

	examGroups := make([]*model.ExamGroup, 0)

	findOptions := options.Find()
	findOptions.SetSort(bson.D{{Key: "examGroupCode", Value: 1}})

	cur, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("collection", collectionNameExamGroups).Msg("MongoDB Find")
		return examGroups, err
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var examGroup model.ExamGroup

		err := cur.Decode(&examGroup)
		if err != nil {
			log.Error().Err(err).Str("semester", db.semester).Str("collection", collectionNameExamGroups).Interface("cur", cur).
				Msg("Cannot decode to additional exam")
			return examGroups, err
		}

		examGroups = append(examGroups, &examGroup)
	}

	if err := cur.Err(); err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("collection", collectionNameExamGroups).Msg("Cursor returned error")
		return examGroups, err
	}

	return examGroups, nil
}
