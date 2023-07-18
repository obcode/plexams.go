package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

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
		examsToInsert = append(examsToInsert, db.modelConnectedExamToConnectedExam(exam))
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

	connectedExam := db.modelConnectedExamToConnectedExam(exam)
	_, err := collection.ReplaceOne(ctx, bson.D{{Key: "zpaExamAncode", Value: exam.ZpaExam.AnCode}}, connectedExam)
	if err != nil {
		log.Error().Err(err).Int("zpaexam.ancode", exam.ZpaExam.AnCode).Msg("cannot replace connected exam")
		return err
	}

	return nil
}

func (db *DB) GetConnectedExam(ctx context.Context, ancode int) (*model.ConnectedExam, error) {
	collection := db.Client.Database(db.databaseName).Collection(collectionNameConnectedExams)

	var result ConnectedExam
	err := collection.FindOne(ctx, bson.D{{Key: "zpaExamAncode", Value: ancode}}).Decode(&result)
	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).
			Int("ancode", ancode).Msg("cannot find Connected exam")
		return nil, err
	}

	connectedExam, err := db.connectedExamToModelConnectedExam(ctx, &result)
	if err != nil {
		log.Error().Err(err).Int("ancode", result.ZpaExam).Msg("cannot get connected exam")
	}

	return connectedExam, nil
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

	if err != nil {
		log.Error().Err(err).Str("semester", db.semester).Str("collection", collectionNameConnectedExams).Msg("Cursor returned error")
		return nil, err
	}

	if exams == nil {
		return nil, nil
	}

	connectedExams := make([]*model.ConnectedExam, 0, len(exams))
	for _, exam := range exams {
		connectedExam, err := db.connectedExamToModelConnectedExam(ctx, exam)
		if err != nil {
			log.Error().Err(err).Int("ancode", exam.ZpaExam).Msg("cannot get connected exam")
		}
		connectedExams = append(connectedExams, connectedExam)
	}

	return connectedExams, nil
}

func (db *DB) modelConnectedExamToConnectedExam(exam *model.ConnectedExam) *ConnectedExam {
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

	return &ConnectedExam{
		ZpaExam:           exam.ZpaExam.AnCode,
		PrimussExams:      primussExamsAncodes,
		OtherPrimussExams: otherPrimussExamsAncodes,
		Errors:            exam.Errors,
	}
}

func (db *DB) connectedExamToModelConnectedExam(ctx context.Context, exam *ConnectedExam) (*model.ConnectedExam, error) {
	zpaExam, err := db.GetZpaExamByAncode(ctx, exam.ZpaExam)
	if err != nil {
		log.Error().Err(err).Int("ancode", exam.ZpaExam).Msg("cannot get zpa exam")
		return nil, err
	}

	var primussExams []*model.PrimussExam
	for _, exam := range exam.PrimussExams {
		primussExam, err := db.GetPrimussExam(ctx, exam.Program, exam.Ancode)
		if err != nil {
			log.Error().Err(err).Int("ancode", exam.Ancode).Str("program", exam.Program).Msg("cannot get primuss exam")
			return nil, err
		}
		primussExams = append(primussExams, primussExam)
	}

	var otherPrimussExams []*model.PrimussExam
	for _, exam := range exam.OtherPrimussExams {
		primussExam, err := db.GetPrimussExam(ctx, exam.Program, exam.Ancode)
		if err != nil {
			log.Error().Err(err).Int("ancode", exam.Ancode).Str("program", exam.Program).Msg("cannot get primuss exam")
			return nil, err
		}
		otherPrimussExams = append(otherPrimussExams, primussExam)
	}

	return &model.ConnectedExam{
		ZpaExam:           zpaExam,
		PrimussExams:      primussExams,
		OtherPrimussExams: otherPrimussExams,
		Errors:            exam.Errors,
	}, nil
}
