package db

import (
	"context"
	"sort"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

var collectionNamePlan = "plan"

func (db *DB) AddExamGroupToSlot(ctx context.Context, dayNumber int, timeNumber int, examGroupCode int) (bool, error) {
	collection := db.Client.Database(databaseName(db.semester)).Collection(collectionNamePlan)

	_, err := collection.DeleteMany(ctx, bson.D{{Key: "examgroupcode", Value: examGroupCode}})
	if err != nil {
		log.Error().Err(err).Int("day", dayNumber).Int("time", timeNumber).Int("examGroupCode", examGroupCode).
			Msg("cannot rm exam group from plan")
		return false, err
	}

	_, err = collection.InsertOne(ctx, &model.PlanEntry{
		DayNumber:     dayNumber,
		SlotNumber:    timeNumber,
		ExamGroupCode: examGroupCode,
	})

	if err != nil {
		log.Error().Err(err).Int("day", dayNumber).Int("time", timeNumber).Int("examGroupCode", examGroupCode).
			Msg("cannot add exam group to slot")
		return false, err
	}

	return true, nil
}

func (db *DB) ExamGroupsInSlot(ctx context.Context, day int, time int) ([]*model.ExamGroup, error) {
	collection := db.Client.Database(databaseName(db.semester)).Collection(collectionNamePlan)

	filter := bson.M{
		"$and": []bson.M{
			{"daynumber": day},
			{"slotnumber": time},
		},
	}
	cur, err := collection.Find(ctx, filter)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionNamePlan).Msg("MongoDB Find")
		return nil, err
	}
	defer cur.Close(ctx)

	planEntries := make([]*model.PlanEntry, 0)
	for cur.Next(ctx) {
		var planEntry model.PlanEntry

		err := cur.Decode(&planEntry)
		if err != nil {
			log.Error().Err(err).Str("collection", collectionNamePlan).Interface("cur", cur).
				Msg("Cannot decode to nta")
			return nil, err
		}

		planEntries = append(planEntries, &planEntry)
	}

	if err := cur.Err(); err != nil {
		log.Error().Err(err).Str("collection", collectionNamePlan).Msg("Cursor returned error")
		return nil, err
	}

	examGroups := make([]*model.ExamGroup, 0, len(planEntries))
	for _, planEntry := range planEntries {
		examGroup, err := db.ExamGroup(ctx, planEntry.ExamGroupCode)
		if err != nil {
			log.Error().Err(err).Int("examGroupCode", planEntry.ExamGroupCode).Msg("cannot get exam group")
			return examGroups, err
		}
		examGroups = append(examGroups, examGroup)
	}

	return examGroups, nil
}

func (db *DB) PlanEntries(ctx context.Context) ([]*model.PlanEntry, error) {
	collection := db.Client.Database(databaseName(db.semester)).Collection(collectionNamePlan)

	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Str("collection", collectionNamePlan).Msg("MongoDB Find")
		return nil, err
	}
	defer cur.Close(ctx)

	planEntries := make([]*model.PlanEntry, 0)
	for cur.Next(ctx) {
		var planEntry model.PlanEntry

		err := cur.Decode(&planEntry)
		if err != nil {
			log.Error().Err(err).Str("collection", collectionNamePlan).Interface("cur", cur).
				Msg("Cannot decode to plan entry")
			return nil, err
		}

		planEntries = append(planEntries, &planEntry)
	}

	return planEntries, nil
}

func (db *DB) PlanEntryForExamGroup(ctx context.Context, examGroupCode int) (*model.PlanEntry, error) {
	collection := db.Client.Database(databaseName(db.semester)).Collection(collectionNamePlan)

	res := collection.FindOne(ctx, bson.D{{Key: "examgroupcode", Value: examGroupCode}})
	if res.Err() != nil {
		if res.Err() == mongo.ErrNoDocuments {
			return nil, nil
		}
		log.Error().Err(res.Err()).Str("collection", collectionNamePlan).Msg("MongoDB Find")
		return nil, res.Err()
	}
	var entry model.PlanEntry

	err := res.Decode(&entry)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionNamePlan).
			Msg("Cannot decode to plan entry")
		return nil, err
	}

	return &entry, nil
}

func (db *DB) AncodesInPlan(ctx context.Context) ([]int, error) {
	examGroups, err := db.ExamGroups(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get exam groups")
	}

	ancodes := make([]int, 0)

	for _, group := range examGroups {
		for _, exam := range group.Exams {
			ancodes = append(ancodes, exam.Exam.Ancode)
		}
	}

	sort.Ints(ancodes)
	return ancodes, nil
}

func (db *DB) ExamerInPlan(ctx context.Context) ([]*model.ExamerInPlan, error) {
	examGroups, err := db.ExamGroups(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get exam groups")
	}

	examerMap := make(map[string][]int)
	for _, group := range examGroups {
	EXAMLOOP:
		for _, exam := range group.Exams {
			examer, ok := examerMap[exam.Exam.ZpaExam.MainExamer]
			if !ok {
				examerMap[exam.Exam.ZpaExam.MainExamer] = []int{exam.Exam.ZpaExam.MainExamerID}
			} else {
				for _, examerID := range examer {
					if examerID == exam.Exam.ZpaExam.MainExamerID {
						continue EXAMLOOP
					}
				}
				examer = append(examer, exam.Exam.ZpaExam.MainExamerID)
				examerMap[exam.Exam.ZpaExam.MainExamer] = examer
			}
		}
	}

	names := make([]string, 0, len(examerMap))
	for name, _ := range examerMap {
		names = append(names, name)
	}
	sort.Strings(names)

	examer := make([]*model.ExamerInPlan, 0, len(examerMap))
	for _, name := range names {
		ids := examerMap[name]
		for _, id := range ids {
			examer = append(examer, &model.ExamerInPlan{
				MainExamer:   name,
				MainExamerID: id,
			})
		}
	}

	return examer, nil
}
