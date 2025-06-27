package db

import (
	"context"
	"sort"
	"time"

	set "github.com/deckarep/golang-set/v2"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (db *DB) GetZPAExams(ctx context.Context) ([]*model.ZPAExam, error) {
	collection := db.Client.Database(db.databaseName).Collection(collectionAll)

	exams := make([]*model.ZPAExam, 0)

	findOptions := options.Find()
	findOptions.SetSort(bson.D{{Key: "ancode", Value: 1}})

	cur, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("collection", "zpaexams").Msg("MongoDB Find")
		return exams, err
	}
	defer cur.Close(ctx) //nolint:errcheck

	addedAncodes, err := db.GetAddedAncodes(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get added ancodes")
		return nil, err
	}

	for cur.Next(ctx) {
		var exam model.ZPAExam

		err := cur.Decode(&exam)
		if err != nil {
			log.Error().Err(err).Str("semester", db.semester).Str("collection", "zpaexams").Interface("cur", cur).
				Msg("Cannot decode to exam")
			return exams, err
		}

		db.cleanupPrimussAncodes(&exam)
		addedAncodesForAncode, ok := addedAncodes[exam.AnCode]

		if ok {
			err := db.addAddedAncodesToExam(ctx, &exam, addedAncodesForAncode)
			if err != nil {
				log.Error().Err(err).Int("ancode", exam.AnCode).
					Interface("added ancodes", addedAncodesForAncode).
					Msg("error when trying to add added ancodes to exam")
				return nil, err
			}
		}

		exams = append(exams, &exam)
	}

	if err := cur.Err(); err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("collection", "zpaexams").Msg("Cursor returned error")
		return exams, err
	}

	return exams, nil
}

func (db *DB) GetZpaExamByAncode(ctx context.Context, ancode int) (*model.ZPAExam, error) {
	collection := db.Client.Database(db.databaseName).Collection("zpaexams")

	var result model.ZPAExam
	err := collection.FindOne(ctx, bson.D{{Key: "ancode", Value: ancode}}).Decode(&result)
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).
			Int("ancode", ancode).Msg("cannot find ZPA exam")
		return nil, err
	}

	db.cleanupPrimussAncodes(&result)
	addedAncodes, err := db.GetAddedAncodesForAncode(ctx, result.AnCode)
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).
			Int("ancode", ancode).Msg("cannot get added ancodes")
		return nil, err
	}
	if addedAncodes != nil {
		err := db.addAddedAncodesToExam(ctx, &result, addedAncodes)
		if err != nil {
			log.Error().Err(err).Str("semester", db.semester).
				Int("ancode", ancode).Msg("cannot add added ancodes")
			return nil, err
		}
	}

	return &result, nil
}

func (db *DB) CacheZPAExams(exams []*model.ZPAExam) error {
	collection := db.Client.Database(db.databaseName).Collection("zpaexams")

	examsIntf := make([]interface{}, 0, len(exams))

	for _, v := range exams {
		examsIntf = append(examsIntf, v)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := collection.Drop(ctx)
	if err != nil {
		return err
	}

	res, err := collection.InsertMany(ctx, examsIntf)
	if err != nil {
		return err
	}

	log.Debug().Str("semester", db.semester).Int("documents", len(res.InsertedIDs)).Msg("inserted zpaexams")

	return nil
}

type ExamToPlanType struct {
	Ancode int
	ToPlan bool
}

func (db *DB) SetZPAExamsToPlan(ctx context.Context, examsToPlan, examsNotToPlan []*model.ZPAExam) error {
	exams := make([]*ExamToPlanType, 0, len(examsToPlan)+len(examsNotToPlan))

	log.Debug().Interface("examsToPlan", examsToPlan).Interface("examsNotToPlan", examsNotToPlan).Msg("inserting exams to plan")

	for _, examToPlan := range examsToPlan {
		exams = append(exams, &ExamToPlanType{Ancode: examToPlan.AnCode, ToPlan: true})
	}

	for _, examNotToPlan := range examsNotToPlan {
		exams = append(exams, &ExamToPlanType{Ancode: examNotToPlan.AnCode, ToPlan: false})
	}

	collection := db.Client.Database(db.databaseName).Collection(collectionToPlan)

	err := collection.Drop(ctx)
	if err != nil {
		return err
	}

	log.Debug().Interface("exams", exams).Msg("inserting exams to plan")

	examsIntf := make([]interface{}, 0, len(exams))
	for _, exam := range exams {
		examsIntf = append(examsIntf, exam)
	}

	res, err := collection.InsertMany(ctx, examsIntf)
	if err != nil {
		return err
	}

	log.Debug().Str("semester", db.semester).Str("collection", collectionToPlan).
		Int("documents", len(res.InsertedIDs)).Msg("inserted zpaexams to plan and not to plan")

	return nil
}

func (db *DB) AddZpaExamToPlan(ctx context.Context, ancode int) (bool, error) {
	return db.addZpaExamToPlanOrNot(ctx, ancode, true)
}

func (db *DB) RmZpaExamFromPlan(ctx context.Context, ancode int) (bool, error) {
	return db.addZpaExamToPlanOrNot(ctx, ancode, false)
}

func (db *DB) addZpaExamToPlanOrNot(ctx context.Context, ancode int, toPlan bool) (bool, error) {
	collection := db.Client.Database(db.databaseName).Collection(collectionToPlan)

	replaceOptions := options.Replace()
	replaceOptions.SetUpsert(true)

	res, err := collection.ReplaceOne(ctx, bson.D{{Key: "ancode", Value: ancode}},
		ExamToPlanType{Ancode: ancode, ToPlan: toPlan}, replaceOptions)

	log.Debug().Interface("res", res).Msg("changing exam to plan value")

	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Bool("toPlan", toPlan).Msg("cannot replace exam to plan")
		return false, err
	}

	return true, nil
}

func (db *DB) GetZPAExamsToPlan(ctx context.Context) ([]*model.ZPAExam, error) {
	log.Debug().Msg("getting zpa exams to plan")
	toPlan := true
	return db.getZPAExamsPlannedOrNot(ctx, &toPlan)
}

func (db *DB) GetZPAExamsNotToPlan(ctx context.Context) ([]*model.ZPAExam, error) {
	log.Debug().Msg("getting zpa exams not to plan")
	toPlan := false
	return db.getZPAExamsPlannedOrNot(ctx, &toPlan)
}

func (db *DB) GetZPAExamsPlannedOrNotPlanned(ctx context.Context) ([]*model.ZPAExam, error) {
	return db.getZPAExamsPlannedOrNot(ctx, nil)
}

func (db *DB) getZPAExamsPlannedOrNot(ctx context.Context, toPlan *bool) ([]*model.ZPAExam, error) {
	log.Debug().Interface("toPlan", toPlan).Msg("getting zpam exams")

	ancodeSet, err := db.getZpaAncodesPlannedOrNot(ctx, toPlan)
	if err != nil {
		log.Error().Err(err).Msg("cannot get ancodes planned")
		return nil, err
	}

	zpaExams, err := db.GetZPAExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get zpa exams")
		return nil, err
	}

	addedAncodes, err := db.GetAddedAncodes(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get added ancodes")
		return nil, err
	}

	exams := make([]*model.ZPAExam, 0, (*ancodeSet).Cardinality())

	for _, zpaExam := range zpaExams {
		if (*ancodeSet).Contains(zpaExam.AnCode) {
			db.cleanupPrimussAncodes(zpaExam)
			addedAncodesForAncode, ok := addedAncodes[zpaExam.AnCode]

			if ok {
				err := db.addAddedAncodesToExam(ctx, zpaExam, addedAncodesForAncode)
				if err != nil {
					log.Error().Err(err).Int("ancode", zpaExam.AnCode).
						Interface("added ancodes", addedAncodesForAncode).
						Msg("error when trying to add added ancodes to exam")
					return nil, err
				}
			}

			exams = append(exams, zpaExam)
		}
	}

	return exams, nil
}

func (db *DB) GetZpaAncodesPlanned(ctx context.Context) (*set.Set[int], error) {
	toPlan := true
	return db.getZpaAncodesPlannedOrNot(ctx, &toPlan)
}

func (db *DB) GetZpaAncodesNotPlanned(ctx context.Context) (*set.Set[int], error) {
	toPlan := false
	return db.getZpaAncodesPlannedOrNot(ctx, &toPlan)
}

func (db *DB) GetZpaAncodesPlannedOrNotPlanned(ctx context.Context) (*set.Set[int], error) {
	return db.getZpaAncodesPlannedOrNot(ctx, nil)
}

func (db *DB) getZpaAncodesPlannedOrNot(ctx context.Context, toPlan *bool) (*set.Set[int], error) {
	collection := db.Client.Database(db.databaseName).Collection(collectionToPlan)

	filter := bson.D{}
	if toPlan != nil {
		filter = bson.D{{Key: "toplan", Value: toPlan}}
	}

	cur, err := collection.Find(ctx, filter)
	if err != nil {
		log.Error().Err(err).Interface("toPlan", toPlan).Msg("cannot get zpa exams to plan")
		return nil, err
	}
	defer cur.Close(ctx) //nolint:errcheck

	result := make([]*ExamToPlanType, 0)

	err = cur.All(ctx, &result)
	if err != nil {
		log.Error().Err(err).Interface("toPlan", toPlan).Msg("cannot decode from cursor")
		return nil, err
	}

	resultSet := set.NewSet[int]()
	for _, examToPlan := range result {
		resultSet.Add(examToPlan.Ancode)
	}

	return &resultSet, nil
}

func (db *DB) cleanupPrimussAncodes(zpaExam *model.ZPAExam) {
	programs := set.NewSet[string]()

	ancodesMap := make(map[string]int)
	for _, group := range zpaExam.Groups {
		ancodesMap[group[:2]] = -1
		programs.Add(group[:2])
	}

	for _, primussAncode := range zpaExam.PrimussAncodes {
		if programs.Contains(primussAncode.Program) {
			ancodesMap[primussAncode.Program] = primussAncode.Ancode
		}
	}

	programSlice := programs.ToSlice()
	sort.Strings(programSlice)

	newPrimussAncodes := make([]model.ZPAPrimussAncodes, 0, len(ancodesMap))

	for _, program := range programSlice {
		newPrimussAncodes = append(newPrimussAncodes, model.ZPAPrimussAncodes{
			Program: program,
			Ancode:  ancodesMap[program],
		})
	}

	zpaExam.PrimussAncodes = newPrimussAncodes
}

func (db *DB) addAddedAncodesToExam(ctx context.Context, zpaExam *model.ZPAExam, addedAncodesForAncode []model.ZPAPrimussAncodes) error {
	if addedAncodesForAncode == nil {
		var err error
		addedAncodesForAncode, err = db.GetAddedAncodesForAncode(ctx, zpaExam.AnCode)
		if err != nil {
			log.Error().Err(err).Int("ancode", zpaExam.AnCode).Msg("cannot get added ancodes")
			return err
		}
		if len(addedAncodesForAncode) == 0 {
			return nil
		}
	}

	allPrimussAncodes := append(zpaExam.PrimussAncodes, addedAncodesForAncode...)

	ancodesMap := make(map[string]int)
	programs := set.NewSet[string]()
	for _, primussAncode := range allPrimussAncodes {
		ancodesMap[primussAncode.Program] = primussAncode.Ancode
		programs.Add(primussAncode.Program)
	}

	programSlice := programs.ToSlice()
	sort.Strings(programSlice)

	newPrimussAncodes := make([]model.ZPAPrimussAncodes, 0, len(ancodesMap))

	for _, program := range programSlice {
		newPrimussAncodes = append(newPrimussAncodes, model.ZPAPrimussAncodes{
			Program: program,
			Ancode:  ancodesMap[program],
		})
	}

	zpaExam.PrimussAncodes = newPrimussAncodes

	return nil
}
