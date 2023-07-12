package db

import (
	"context"
	"fmt"
	"sort"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func (db *DB) AddExamGroupToSlot(ctx context.Context, dayNumber int, timeNumber int, examGroupCode int) (bool, error) {
	if db.ExamGroupIsLocked(ctx, examGroupCode) {
		return false, fmt.Errorf("exam group %d is locked", examGroupCode)
	}

	collection := db.Client.Database(db.databaseName).Collection(collectionNamePlan)

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

func (db *DB) RmExamGroupFromSlot(ctx context.Context, examGroupCode int) (bool, error) {
	if db.ExamGroupIsLocked(ctx, examGroupCode) {
		return false, fmt.Errorf("exam group %d is locked", examGroupCode)
	}

	collection := db.Client.Database(db.databaseName).Collection(collectionNamePlan)

	_, err := collection.DeleteMany(ctx, bson.D{{Key: "examgroupcode", Value: examGroupCode}})
	if err != nil {
		log.Error().Err(err).Int("examGroupCode", examGroupCode).
			Msg("cannot rm exam group from plan")
		return false, err
	}

	return true, nil
}

func (db *DB) ExamGroupsInSlot(ctx context.Context, day int, time int) ([]*model.ExamGroup, error) {
	collection := db.Client.Database(db.databaseName).Collection(collectionNamePlan)

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
	collection := db.Client.Database(db.databaseName).Collection(collectionNamePlan)

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
	collection := db.Client.Database(db.databaseName).Collection(collectionNamePlan)

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
	for name := range examerMap {
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

func (db *DB) PlanAncodeEntries(ctx context.Context) ([]*model.PlanAncodeEntry, error) {
	examGroups, err := db.ExamGroups(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get exam groups")
	}

	examGroupMap := make(map[int]*model.ExamGroup)
	for _, examGroup := range examGroups {
		examGroupMap[examGroup.ExamGroupCode] = examGroup
	}

	planEntries, err := db.PlanEntries(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get plan entries")
	}

	planAncodeEntries := make([]*model.PlanAncodeEntry, 0)
	for _, planEntry := range planEntries {
		examGroup, ok := examGroupMap[planEntry.ExamGroupCode]
		if !ok {
			log.Error().Int("exam group code", planEntry.ExamGroupCode).Msg("exam group not found")
		}
		for _, exam := range examGroup.Exams {
			planAncodeEntries = append(planAncodeEntries, &model.PlanAncodeEntry{
				DayNumber:  planEntry.DayNumber,
				SlotNumber: planEntry.SlotNumber,
				Ancode:     exam.Exam.Ancode,
			})
		}
	}
	return planAncodeEntries, nil
}

func (db *DB) LockExamGroup(ctx context.Context, examGroupCode int) (*model.PlanEntry, error) {
	_, err := db.PlanEntryForExamGroup(ctx, examGroupCode)
	if err != nil {
		log.Error().Err(err).Int("exam group code", examGroupCode).
			Msg("cannot find plan entry")
		return nil, err
	}

	collection := db.Client.Database(db.databaseName).Collection(collectionNamePlan)

	filter := bson.D{{Key: "examgroupcode", Value: examGroupCode}}
	update := bson.D{{Key: "$set", Value: bson.D{{Key: "locked", Value: true}}}}

	_, err = collection.UpdateOne(ctx, filter, update)
	if err != nil {
		log.Error().Err(err).Int("examgroupcode", examGroupCode).
			Msg("cannot lock exam group to slot")
		return nil, err
	}
	return db.PlanEntryForExamGroup(ctx, examGroupCode)
}

func (db *DB) UnlockExamGroup(ctx context.Context, examGroupCode int) (*model.PlanEntry, error) {
	_, err := db.PlanEntryForExamGroup(ctx, examGroupCode)
	if err != nil {
		log.Error().Err(err).Int("exam group code", examGroupCode).
			Msg("cannot find plan entry")
		return nil, err
	}

	collection := db.Client.Database(db.databaseName).Collection(collectionNamePlan)

	filter := bson.D{{Key: "examgroupcode", Value: examGroupCode}}
	update := bson.D{{Key: "$set", Value: bson.D{{Key: "locked", Value: false}}}}

	_, err = collection.UpdateOne(ctx, filter, update)
	if err != nil {
		log.Error().Err(err).Int("examgroupcode", examGroupCode).
			Msg("cannot unlock exam group")
		return nil, err
	}
	return db.PlanEntryForExamGroup(ctx, examGroupCode)
}

func (db *DB) ExamGroupIsLocked(ctx context.Context, examGroupCode int) bool {
	p, err := db.PlanEntryForExamGroup(ctx, examGroupCode)
	return err == nil && p != nil && p.Locked
}

func (db *DB) RemoveUnlockedExamGroupsFromPlan(ctx context.Context) (int, error) {
	collection := db.Client.Database(db.databaseName).Collection(collectionNamePlan)

	res, err := collection.DeleteMany(ctx, bson.D{{Key: "locked", Value: false}})

	if err != nil {
		log.Error().Err(err).Msg("error while trying to delete all unlocked exam groups from the plan")
		return 0, err
	}

	log.Debug().Int64("count", res.DeletedCount).Msg("deleted exam groups")

	return int(res.DeletedCount), nil
}

func (db *DB) LockPlan(ctx context.Context) error {
	collection := db.Client.Database(db.databaseName).Collection(collectionNamePlan)

	res, err := collection.UpdateMany(ctx, bson.D{},
		bson.D{{Key: "$set", Value: bson.D{{Key: "locked", Value: true}}}})

	if err != nil {
		log.Error().Err(err).Msg("error while trying to lock the plan")
		return err
	}

	log.Debug().Int64("count", res.ModifiedCount).Msg("locked exam groups")

	return nil
}

func (db *DB) SavePlanEntries(ctx context.Context, planEntries []*model.PlanEntry) error {
	log.Debug().Msg("saving plan entries to plan")
	return db.savePlanEntries(ctx, planEntries, false)
}

func (db *DB) SavePlanEntriesToBackup(ctx context.Context, planEntries []*model.PlanEntry) error {
	log.Debug().Msg("saving plan entries to backup plan")
	return db.savePlanEntries(ctx, planEntries, true)
}

func (db *DB) savePlanEntries(ctx context.Context, planEntries []*model.PlanEntry, backup bool) error {
	var collection *mongo.Collection
	if backup {
		collection = db.Client.Database(db.databaseName).Collection(collectionNamePlanBackup)
		err := collection.Drop(ctx)
		if err != nil {
			log.Error().Err(err).Msg("cannot drop backup collection")
		}
	} else {
		collection = db.Client.Database(db.databaseName).Collection(collectionNamePlan)
	}

	entries := make([]interface{}, 0, len(planEntries))
	for _, entry := range planEntries {
		entries = append(entries, entry)
	}

	res, err := collection.InsertMany(ctx, entries)
	if err != nil {
		log.Error().Err(err).Bool("backup", backup).Msg("cannot insert entries to plan")
		return err
	}

	log.Debug().Bool("backup", backup).Int("count", len(res.InsertedIDs)).Msg("inserted entries to plan")
	return nil
}

func (db *DB) BackupPlan(ctx context.Context) error {
	planEntries, err := db.PlanEntries(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get plan entries")
		return err
	}

	return db.SavePlanEntriesToBackup(ctx, planEntries)
}
