package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
)

func (db *DB) CacheGeneratedExams(ctx context.Context, exams []*model.GeneratedExam) error {
	collection := db.Client.Database(db.databaseName).Collection(collectionGeneratedExams)

	err := collection.Drop(ctx)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionGeneratedExams).Msg("cannot drop collection")
		return err
	}

	examsIntf := make([]interface{}, 0, len(exams))
	for _, exam := range exams {
		examsIntf = append(examsIntf, exam)
	}

	res, err := collection.InsertMany(ctx, examsIntf)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionGeneratedExams).Msg("cannot insert generated exams")
		return err
	}

	log.Debug().Int("count", len(res.InsertedIDs)).Msg("successfully inserted generated exams")
	return nil
}

func (db *DB) GetGeneratedExams(ctx context.Context) ([]*model.GeneratedExam, error) {
	collection := db.Client.Database(db.databaseName).Collection(collectionGeneratedExams)

	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Msg("cannot get generated exams")
		return nil, err
	}
	defer cur.Close(ctx) //nolint:errcheck

	exams := make([]*model.GeneratedExam, 0)

	err = cur.All(ctx, &exams)
	if err != nil {
		log.Error().Err(err).Msg("cannot decode generated exams")
		return nil, err
	}

	return exams, nil
}

func (db *DB) GetGeneratedExamsForExamer(ctx context.Context, examerID int) ([]*model.GeneratedExam, error) {
	collection := db.Client.Database(db.databaseName).Collection(collectionGeneratedExams)

	cur, err := collection.Find(ctx, bson.D{{Key: "zpaexam.mainexamerid", Value: examerID}})
	if err != nil {
		log.Error().Err(err).Msg("cannot get generated exams")
		return nil, err
	}
	defer cur.Close(ctx) //nolint:errcheck

	exams := make([]*model.GeneratedExam, 0)

	err = cur.All(ctx, &exams)
	if err != nil {
		log.Error().Err(err).Msg("cannot decode generated exams")
		return nil, err
	}

	return exams, nil
}

func (db *DB) GetGeneratedExam(ctx context.Context, ancode int) (*model.GeneratedExam, error) {
	collection := db.Client.Database(db.databaseName).Collection(collectionGeneratedExams)

	res := collection.FindOne(ctx, bson.D{{Key: "ancode", Value: ancode}})
	if res.Err() != nil {
		log.Error().Err(res.Err()).Int("ancode", ancode).Msg("cannot get generated exam")
		return nil, res.Err()
	}

	var exam *model.GeneratedExam

	err := res.Decode(&exam)
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot get generated exam")
		return nil, err
	}

	return exam, nil
}
