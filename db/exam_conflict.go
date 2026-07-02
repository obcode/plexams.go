package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// conflictRatingDoc is the stored form of a conflict rating. Mtknr "" means a
// pair-level rating (UNDESIRED/FORBIDDEN); a non-empty Mtknr is a per-student
// ACCEPTED rating. Key: (ancode1, ancode2, mtknr).
type conflictRatingDoc struct {
	Ancode1 int    `bson:"ancode1"`
	Ancode2 int    `bson:"ancode2"`
	Rating  string `bson:"rating"`
	Mtknr   string `bson:"mtknr"`
}

// ConflictRatings returns all stored conflict ratings.
func (db *DB) ConflictRatings(ctx context.Context) ([]*model.ExamConflictRating, error) {
	collection := db.getCollectionSemester(collectionExamConflictRatings)
	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Msg("cannot get conflict ratings")
		return nil, err
	}
	var docs []conflictRatingDoc
	if err := cur.All(ctx, &docs); err != nil {
		log.Error().Err(err).Msg("cannot decode conflict ratings")
		return nil, err
	}
	out := make([]*model.ExamConflictRating, 0, len(docs))
	for _, d := range docs {
		r := &model.ExamConflictRating{Ancode1: d.Ancode1, Ancode2: d.Ancode2, Rating: model.ConflictRating(d.Rating)}
		if d.Mtknr != "" {
			m := d.Mtknr
			r.Mtknr = &m
		}
		out = append(out, r)
	}
	return out, nil
}

// UpsertConflictRating stores (or replaces) a conflict rating by (ancode1, ancode2,
// mtknr). Pass mtknr "" for a pair-level rating.
func (db *DB) UpsertConflictRating(ctx context.Context, ancode1, ancode2 int, rating, mtknr string) error {
	collection := db.getCollectionSemester(collectionExamConflictRatings)
	filter := bson.M{"ancode1": ancode1, "ancode2": ancode2, "mtknr": mtknr}
	doc := conflictRatingDoc{Ancode1: ancode1, Ancode2: ancode2, Rating: rating, Mtknr: mtknr}
	if _, err := collection.ReplaceOne(ctx, filter, doc, options.Replace().SetUpsert(true)); err != nil {
		log.Error().Err(err).Msg("cannot upsert conflict rating")
		return err
	}
	return nil
}

// DeleteConflictRating removes a conflict rating by (ancode1, ancode2, mtknr).
func (db *DB) DeleteConflictRating(ctx context.Context, ancode1, ancode2 int, mtknr string) (bool, error) {
	collection := db.getCollectionSemester(collectionExamConflictRatings)
	res, err := collection.DeleteOne(ctx, bson.M{"ancode1": ancode1, "ancode2": ancode2, "mtknr": mtknr})
	if err != nil {
		log.Error().Err(err).Msg("cannot delete conflict rating")
		return false, err
	}
	return res.DeletedCount > 0, nil
}

type canShareDoc struct {
	Ancode1 int `bson:"ancode1"`
	Ancode2 int `bson:"ancode2"`
}

// CanShareSlotPairs returns the exam pairs declared as allowed to share a slot.
func (db *DB) CanShareSlotPairs(ctx context.Context) ([][2]int, error) {
	collection := db.getCollectionSemester(collectionExamCanShareSlot)
	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Msg("cannot get can-share-slot pairs")
		return nil, err
	}
	var docs []canShareDoc
	if err := cur.All(ctx, &docs); err != nil {
		log.Error().Err(err).Msg("cannot decode can-share-slot pairs")
		return nil, err
	}
	out := make([][2]int, 0, len(docs))
	for _, d := range docs {
		out = append(out, [2]int{d.Ancode1, d.Ancode2})
	}
	return out, nil
}

// UpsertCanShareSlot declares that two exams may share a slot (key: ancode1, ancode2).
func (db *DB) UpsertCanShareSlot(ctx context.Context, ancode1, ancode2 int) error {
	collection := db.getCollectionSemester(collectionExamCanShareSlot)
	filter := bson.M{"ancode1": ancode1, "ancode2": ancode2}
	if _, err := collection.ReplaceOne(ctx, filter, canShareDoc{ancode1, ancode2}, options.Replace().SetUpsert(true)); err != nil {
		log.Error().Err(err).Msg("cannot upsert can-share-slot pair")
		return err
	}
	return nil
}

// DeleteCanShareSlot removes a can-share-slot declaration.
func (db *DB) DeleteCanShareSlot(ctx context.Context, ancode1, ancode2 int) (bool, error) {
	collection := db.getCollectionSemester(collectionExamCanShareSlot)
	res, err := collection.DeleteOne(ctx, bson.M{"ancode1": ancode1, "ancode2": ancode2})
	if err != nil {
		log.Error().Err(err).Msg("cannot delete can-share-slot pair")
		return false, err
	}
	return res.DeletedCount > 0, nil
}
