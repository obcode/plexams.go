package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
)

// ReplaceUnplacedExams drops and rewrites the rooms_unplaced collection (the
// students that could not be assigned a real room in their slot during room
// generation).
func (db *DB) ReplaceUnplacedExams(ctx context.Context, unplaced []*model.UnplacedExam) error {
	collection := db.getCollectionSemester(collectionRoomsUnplaced)

	if err := collection.Drop(ctx); err != nil {
		log.Error().Err(err).Msg("cannot drop unplaced exams")
		return err
	}

	if len(unplaced) == 0 {
		return nil
	}

	docs := make([]interface{}, 0, len(unplaced))
	for _, u := range unplaced {
		docs = append(docs, u)
	}

	if _, err := collection.InsertMany(ctx, docs); err != nil {
		log.Error().Err(err).Msg("cannot insert unplaced exams")
		return err
	}

	return nil
}

// UnplacedExams returns the students that could not be assigned a real room in
// their slot during the last room generation.
func (db *DB) UnplacedExams(ctx context.Context) ([]*model.UnplacedExam, error) {
	collection := db.getCollectionSemester(collectionRoomsUnplaced)

	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Msg("cannot find unplaced exams")
		return nil, err
	}

	unplaced := make([]*model.UnplacedExam, 0)
	if err := cur.All(ctx, &unplaced); err != nil {
		log.Error().Err(err).Msg("cannot decode unplaced exams")
		return nil, err
	}
	for _, ue := range unplaced {
		db.decorateUnplacedExam(ue)
	}

	return unplaced, nil
}

// ResetUnplacedExams drops the rooms_unplaced collection.
func (db *DB) ResetUnplacedExams(ctx context.Context) error {
	collection := db.getCollectionSemester(collectionRoomsUnplaced)
	if err := collection.Drop(ctx); err != nil {
		log.Error().Err(err).Msg("cannot drop unplaced exams")
		return err
	}
	return nil
}
