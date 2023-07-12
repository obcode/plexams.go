package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (db *DB) GetNextExamGroupCode(ctx context.Context) (int, error) {
	collection := db.Client.Database(db.databaseName).Collection(collectionNameExamGroups)

	filter := bson.D{}
	opts := options.Find().SetSort(bson.D{{Key: "examgroupcode", Value: -1}}).SetLimit(1)

	cur, err := collection.Find(ctx, filter, opts)
	if err != nil {
		log.Error().Err(err).Msg("cannot find the highest exam group code")
	}

	if cur.Next(ctx) {
		value := cur.Current.Lookup("examgroupcode").AsInt64()
		err = cur.Close(ctx)
		if err != nil {
			log.Error().Err(err).Msg("cannot close the cursor")
		}
		return int(value) + 1, nil
	}

	if err := cur.Err(); err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("collection", collectionNameExamGroups).Msg("Cursor returned error")
		return -1, err
	}

	return -1, nil

}

func (db *DB) SaveExamGroups(ctx context.Context, exams []*model.ExamGroup) error {
	collection := db.Client.Database(db.databaseName).Collection(collectionNameExamGroups)

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
	collection := db.Client.Database(db.databaseName).Collection(collectionNameExamGroups)

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
	collection := db.Client.Database(db.databaseName).Collection(collectionNameExamGroups)

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

func (db *DB) GetAncodesPlannedPerProgram(ctx context.Context) (map[int][]string, error) {
	connectedExams, err := db.GetConnectedExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get connected exams")
		return nil, err
	}

	ancodesAndPrograms := make(map[int][]string)

	for _, connectedExam := range connectedExams {
		programs := make([]string, 0, len(connectedExam.PrimussExams))
		for _, primussExam := range connectedExam.PrimussExams {
			programs = append(programs, primussExam.Program)
		}
		ancodesAndPrograms[connectedExam.ZpaExam.AnCode] = programs
	}

	return ancodesAndPrograms, nil
}
