package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
)

func (db *DB) CacheAssembledExams(ctx context.Context, exams []*model.AssembledExam) error {
	collection := db.Client.Database(db.databaseName).Collection(collectionAssembledExams)

	err := collection.Drop(ctx)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionAssembledExams).Msg("cannot drop collection")
		return err
	}

	examsIntf := make([]interface{}, 0, len(exams))
	for _, exam := range exams {
		examsIntf = append(examsIntf, exam)
	}

	res, err := collection.InsertMany(ctx, examsIntf)
	if err != nil {
		log.Error().Err(err).Str("collection", collectionAssembledExams).Msg("cannot insert assembled exams")
		return err
	}

	log.Debug().Int("count", len(res.InsertedIDs)).Msg("successfully inserted assembled exams")
	return nil
}

// CountAssembledExams returns how many assembled exams are currently cached (0 before
// the first generation).
func (db *DB) CountAssembledExams(ctx context.Context) (int64, error) {
	return db.Client.Database(db.databaseName).Collection(collectionAssembledExams).CountDocuments(ctx, bson.M{})
}

func (db *DB) GetAssembledExams(ctx context.Context) ([]*model.AssembledExam, error) {
	collection := db.Client.Database(db.databaseName).Collection(collectionAssembledExams)

	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Msg("cannot get assembled exams")
		return nil, err
	}
	defer cur.Close(ctx) //nolint:errcheck

	exams := make([]*model.AssembledExam, 0)

	err = cur.All(ctx, &exams)
	if err != nil {
		log.Error().Err(err).Msg("cannot decode assembled exams")
		return nil, err
	}

	return exams, nil
}

func (db *DB) GetAssembledExamsForExamer(ctx context.Context, examerID int) ([]*model.AssembledExam, error) {
	collection := db.Client.Database(db.databaseName).Collection(collectionAssembledExams)

	cur, err := collection.Find(ctx, bson.D{{Key: "zpaexam.mainexamerid", Value: examerID}})
	if err != nil {
		log.Error().Err(err).Msg("cannot get assembled exams")
		return nil, err
	}
	defer cur.Close(ctx) //nolint:errcheck

	exams := make([]*model.AssembledExam, 0)

	err = cur.All(ctx, &exams)
	if err != nil {
		log.Error().Err(err).Msg("cannot decode assembled exams")
		return nil, err
	}

	return exams, nil
}

func (db *DB) GetAssembledExam(ctx context.Context, ancode int) (*model.AssembledExam, error) {
	collection := db.Client.Database(db.databaseName).Collection(collectionAssembledExams)

	res := collection.FindOne(ctx, bson.D{{Key: "ancode", Value: ancode}})
	if res.Err() != nil {
		log.Debug().Err(res.Err()).Int("ancode", ancode).Msg("cannot get assembled exam")
		return nil, res.Err()
	}

	var exam *model.AssembledExam

	err := res.Decode(&exam)
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot get assembled exam")
		return nil, err
	}

	return exam, nil
}
