package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// PreplanExams returns all SEB/EXaHM pre-planning pseudo-exams of this semester,
// sorted by id.
func (db *DB) PreplanExams(ctx context.Context) ([]*model.PreplanExam, error) {
	collection := db.getCollectionSemester(collectionPreplanExams)

	cur, err := collection.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "id", Value: 1}}))
	if err != nil {
		log.Error().Err(err).Msg("cannot find pre-exams")
		return nil, err
	}
	preplanExams := make([]*model.PreplanExam, 0)
	if err := cur.All(ctx, &preplanExams); err != nil {
		log.Error().Err(err).Msg("cannot decode pre-exams")
		return nil, err
	}
	return preplanExams, nil
}

// PreplanExam returns one pre-exam by id, or nil when none.
func (db *DB) PreplanExam(ctx context.Context, id int) (*model.PreplanExam, error) {
	collection := db.getCollectionSemester(collectionPreplanExams)

	var preplanExam model.PreplanExam
	err := collection.FindOne(ctx, bson.M{"id": id}).Decode(&preplanExam)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		log.Error().Err(err).Int("id", id).Msg("cannot get pre-exam")
		return nil, err
	}
	return &preplanExam, nil
}

// nextPreplanExamID returns max(existing id)+1 (starting at 1).
func (db *DB) nextPreplanExamID(ctx context.Context) (int, error) {
	collection := db.getCollectionSemester(collectionPreplanExams)

	var last model.PreplanExam
	err := collection.FindOne(ctx, bson.M{}, options.FindOne().SetSort(bson.D{{Key: "id", Value: -1}})).Decode(&last)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return 1, nil
		}
		return 0, err
	}
	return last.ID + 1, nil
}

// InsertPreplanExam assigns the next id and inserts the pre-exam.
func (db *DB) InsertPreplanExam(ctx context.Context, preplanExam *model.PreplanExam) (*model.PreplanExam, error) {
	id, err := db.nextPreplanExamID(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot determine next pre-exam id")
		return nil, err
	}
	preplanExam.ID = id

	collection := db.getCollectionSemester(collectionPreplanExams)
	if _, err := collection.InsertOne(ctx, preplanExam); err != nil {
		log.Error().Err(err).Msg("cannot insert pre-exam")
		return nil, err
	}
	return preplanExam, nil
}

// ReplacePreplanExam replaces the pre-exam with the same id. Returns false if there
// was none.
func (db *DB) ReplacePreplanExam(ctx context.Context, preplanExam *model.PreplanExam) (bool, error) {
	collection := db.getCollectionSemester(collectionPreplanExams)

	res, err := collection.ReplaceOne(ctx, bson.M{"id": preplanExam.ID}, preplanExam)
	if err != nil {
		log.Error().Err(err).Int("id", preplanExam.ID).Msg("cannot replace pre-exam")
		return false, err
	}
	return res.MatchedCount > 0, nil
}

// UpsertPreplanExam inserts or replaces a pre-exam keeping its explicit id (unlike
// InsertPreplanExam, which assigns a fresh id). Used by the CSV import so that the id
// references in notSameSlot/canShareSlot stay valid across a re-import.
func (db *DB) UpsertPreplanExam(ctx context.Context, preplanExam *model.PreplanExam) error {
	collection := db.getCollectionSemester(collectionPreplanExams)

	_, err := collection.ReplaceOne(ctx, bson.M{"id": preplanExam.ID}, preplanExam,
		options.Replace().SetUpsert(true))
	if err != nil {
		log.Error().Err(err).Int("id", preplanExam.ID).Msg("cannot upsert pre-exam")
	}
	return err
}

// DeletePreplanExam removes one pre-exam. Returns false if there was none.
func (db *DB) DeletePreplanExam(ctx context.Context, id int) (bool, error) {
	collection := db.getCollectionSemester(collectionPreplanExams)

	res, err := collection.DeleteOne(ctx, bson.M{"id": id})
	if err != nil {
		log.Error().Err(err).Int("id", id).Msg("cannot delete pre-exam")
		return false, err
	}
	return res.DeletedCount > 0, nil
}
