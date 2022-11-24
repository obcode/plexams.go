package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
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
