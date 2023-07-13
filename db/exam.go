package db

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (db *DB) AddAdditionalExam(ctx context.Context, exam model.AdditionalExamInput) (bool, error) {
	collection := db.Client.Database(db.databaseName).Collection(collectionNameAdditionalExams)
	_, err := collection.InsertOne(ctx, exam)
	if err != nil {
		return false, err
	}

	return true, nil
}

func (db *DB) AdditionalExams(ctx context.Context) ([]*model.AdditionalExam, error) {
	collection := db.Client.Database(db.databaseName).Collection(collectionNameAdditionalExams)

	exams := make([]*model.AdditionalExam, 0)

	findOptions := options.Find()
	findOptions.SetSort(bson.D{{Key: "ancode", Value: 1}})

	cur, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("collection", collectionNameAdditionalExams).Msg("MongoDB Find")
		return exams, err
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var exam model.AdditionalExam

		err := cur.Decode(&exam)
		if err != nil {
			log.Error().Err(err).Str("semester", db.semester).Str("collection", collectionNameAdditionalExams).Interface("cur", cur).
				Msg("Cannot decode to additional exam")
			return exams, err
		}

		exams = append(exams, &exam)
	}

	if err := cur.Err(); err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("collection", collectionNameAdditionalExams).Msg("Cursor returned error")
		return exams, err
	}

	return exams, nil
}

func (db *DB) ExamsAlreadyPrepared(ctx context.Context) bool {
	collection := db.Client.Database(db.databaseName).Collection("exams")

	docsCount, err := collection.CountDocuments(ctx, bson.D{})
	if err != nil {
		log.Error().Err(err).Msg("cannot count exams in db")
	}

	return docsCount != 0
}

func (db *DB) AddExam(ctx context.Context, exam *model.Exam) error {
	collection := db.Client.Database(db.databaseName).Collection("exams")

	result := collection.FindOne(ctx, bson.D{{Key: "ancode", Value: exam.AnCode}})
	if result.Err() == nil {
		log.Error().Int("ancode", exam.AnCode).Msg("cannot add exam, exam with ancode already in db")
		return fmt.Errorf("cannot add exam, exam with ancode %d already in db", exam.AnCode)
	}

	if result.Err() == mongo.ErrNoDocuments {
		_, err := collection.InsertOne(ctx, exam)
		if err != nil {
			return err
		}
		return nil
	}

	return result.Err()
}

func (db *DB) SaveConnectedExam(ctx context.Context, exam *model.ConnectedExam) error {
	collection := db.Client.Database(db.databaseName).Collection(collectionNameConnectedExams)

	_, err := collection.InsertOne(ctx, exam)
	if err != nil {
		log.Error().Err(err).
			Str("collectionName", collectionNameConnectedExams).
			Msg("cannot insert exams")
		return err
	}

	return nil
}

type PrimussExam struct {
	Ancode  int    `bson:"ancode"`
	Program string `bson:"program"`
}

type ConnectedExam struct {
	ZpaExam           int            `bson:"zpaExamAncode"`
	PrimussExams      []*PrimussExam `bson:"primussExamsAncodes"`
	OtherPrimussExams []*PrimussExam `bson:"otherPrimussExamsAncodes"`
	Errors            []string       `bson:"errors"`
}

func (db *DB) SaveConnectedExams(ctx context.Context, exams []*model.ConnectedExam) error {
	collection := db.Client.Database(db.databaseName).Collection(collectionNameConnectedExams)

	err := collection.Drop(ctx)
	if err != nil {
		log.Error().Err(err).
			Str("collectionName", collectionNameConnectedExams).
			Msg("cannot drop collection")
		return err
	}

	examsToInsert := make([]interface{}, 0, len(exams))
	for _, exam := range exams {
		var primussExamsAncodes []*PrimussExam
		if exam.PrimussExams != nil && len(exam.PrimussExams) > 0 {
			primussExamsAncodes = make([]*PrimussExam, 0, len(exam.PrimussExams))
			for _, primussExam := range exam.PrimussExams {
				primussExamsAncodes = append(primussExamsAncodes, &PrimussExam{
					Ancode:  primussExam.AnCode,
					Program: primussExam.Program,
				})
			}
		}

		var otherPrimussExamsAncodes []*PrimussExam
		if exam.OtherPrimussExams != nil && len(exam.OtherPrimussExams) > 0 {
			otherPrimussExamsAncodes = make([]*PrimussExam, 0, len(exam.OtherPrimussExams))
			for _, otherPrimussExam := range exam.OtherPrimussExams {
				otherPrimussExamsAncodes = append(otherPrimussExamsAncodes, &PrimussExam{
					Ancode:  otherPrimussExam.AnCode,
					Program: otherPrimussExam.Program,
				})
			}
		}

		examsToInsert = append(examsToInsert, &ConnectedExam{
			ZpaExam:           exam.ZpaExam.AnCode,
			PrimussExams:      primussExamsAncodes,
			OtherPrimussExams: otherPrimussExamsAncodes,
			Errors:            exam.Errors,
		})
	}

	_, err = collection.InsertMany(ctx, examsToInsert)
	if err != nil {
		log.Error().Err(err).
			Str("collectionName", collectionNameConnectedExams).
			Msg("cannot insert exams")
		return err
	}

	return nil
}

func (db *DB) ReplaceConnectedExam(ctx context.Context, exam *model.ConnectedExam) error {
	collection := db.Client.Database(db.databaseName).Collection(collectionNameConnectedExams)

	_, err := collection.ReplaceOne(ctx, bson.D{{Key: "zpaexam.ancode", Value: exam.ZpaExam.AnCode}}, exam)
	if err != nil {
		log.Error().Err(err).Int("zpaexam.ancode", exam.ZpaExam.AnCode).Msg("cannot replace connected exam")
		return err
	}

	return nil
}

func (db *DB) GetConnectedExam(ctx context.Context, ancode int) (*model.ConnectedExam, error) {
	collection := db.Client.Database(db.databaseName).Collection(collectionNameConnectedExams)

	var result model.ConnectedExam
	err := collection.FindOne(ctx, bson.D{{Key: "zpaexam.ancode", Value: ancode}}).Decode(&result)
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).
			Int("ancode", ancode).Msg("cannot find Connected exam")
		return nil, err
	}

	return &result, nil
}

func (db *DB) GetConnectedExams(ctx context.Context) ([]*model.ConnectedExam, error) {
	collection := db.Client.Database(db.databaseName).Collection(collectionNameConnectedExams)

	exams := make([]*ConnectedExam, 0)

	findOptions := options.Find()
	findOptions.SetSort(bson.D{{Key: "zpaExamAncode", Value: 1}})

	cur, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("collection", collectionNameConnectedExams).Msg("MongoDB Find")
		return nil, err
	}
	defer cur.Close(ctx)

	err = cur.All(ctx, &exams)

	if err := cur.Err(); err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("collection", collectionNameConnectedExams).Msg("Cursor returned error")
		return nil, err
	}

	return exams, nil
}

func (db *DB) connectedExamToModelConnectedExam(ctx context.Context, exam *ConnectedExam) *model.ConnectedExam {

}

func (db *DB) SaveExamsWithRegs(ctx context.Context, exams []*model.ExamWithRegs) error {
	collection := db.Client.Database(db.databaseName).Collection(collectionNameExamsWithRegs)

	err := collection.Drop(ctx)
	if err != nil {
		log.Error().Err(err).
			Str("collectionName", collectionNameExamsWithRegs).
			Msg("cannot drop collection")
		return err
	}

	examsToInsert := make([]interface{}, 0, len(exams))
	for _, exam := range exams {
		examsToInsert = append(examsToInsert, exam)
	}

	_, err = collection.InsertMany(ctx, examsToInsert)
	if err != nil {
		log.Error().Err(err).
			Str("collectionName", collectionNameExamsWithRegs).
			Msg("cannot insert exams")
		return err
	}

	return nil
}

func (db *DB) ExamWithRegs(ctx context.Context, ancode int) (*model.ExamWithRegs, error) {
	collection := db.Client.Database(db.databaseName).Collection(collectionNameExamsWithRegs)

	res := collection.FindOne(ctx, bson.D{{Key: "ancode", Value: ancode}})
	if res.Err() != nil {
		log.Error().Err(res.Err()).Int("ancode", ancode).Msg("no constraint found")
		return nil, nil // no constraint available
	}
	var exam model.ExamWithRegs
	err := res.Decode(&exam)
	if err != nil {
		log.Error().Err(res.Err()).Int("ancode", ancode).Msg("cannot decode constraint")
		return nil, err
	}

	return &exam, nil
}

func (db *DB) ExamsWithRegs(ctx context.Context) ([]*model.ExamWithRegs, error) {
	collection := db.Client.Database(db.databaseName).Collection(collectionNameExamsWithRegs)

	exams := make([]*model.ExamWithRegs, 0)

	findOptions := options.Find()
	findOptions.SetSort(bson.D{{Key: "ancode", Value: 1}})

	cur, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("collection", collectionNameExamsWithRegs).Msg("MongoDB Find")
		return exams, err
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var exam model.ExamWithRegs

		err := cur.Decode(&exam)
		if err != nil {
			log.Error().Err(err).Str("semester", db.semester).Str("collection", collectionNameExamsWithRegs).Interface("cur", cur).
				Msg("Cannot decode to additional exam")
			return exams, err
		}

		exams = append(exams, &exam)
	}

	if err := cur.Err(); err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("collection", collectionNameExamsWithRegs).Msg("Cursor returned error")
		return exams, err
	}

	return exams, nil
}

func (db *DB) ExamsInPlan(ctx context.Context) ([]*model.ExamInPlan, error) {
	collection := db.getCollectionSemester(collectionExamsInPlan)

	findOptions := options.Find()
	findOptions.SetSort(bson.D{{Key: "slot.starttime", Value: 1}, {Key: "exam.ancode", Value: 1}})

	cur, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		log.Error().Err(err).Msg("error while trying to find exams in plan")
	}
	defer cur.Close(ctx)

	exams := make([]*model.ExamInPlan, 0)

	err = cur.All(ctx, &exams)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionRooms).Msg("Cannot decode to rooms for exams")
		return nil, err
	}

	return exams, nil
}
