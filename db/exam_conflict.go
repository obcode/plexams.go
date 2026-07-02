package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// acceptanceDoc is a stored per-student conflict acceptance. Key: (ancode1, ancode2,
// mtknr).
type acceptanceDoc struct {
	Ancode1 int    `bson:"ancode1"`
	Ancode2 int    `bson:"ancode2"`
	Mtknr   string `bson:"mtknr"`
}

// StudentConflictAcceptances returns all stored per-student conflict acceptances.
func (db *DB) StudentConflictAcceptances(ctx context.Context) ([]*model.StudentConflictAcceptance, error) {
	collection := db.getCollectionSemester(collectionExamConflictRatings)
	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Msg("cannot get conflict acceptances")
		return nil, err
	}
	var docs []acceptanceDoc
	if err := cur.All(ctx, &docs); err != nil {
		log.Error().Err(err).Msg("cannot decode conflict acceptances")
		return nil, err
	}
	out := make([]*model.StudentConflictAcceptance, 0, len(docs))
	for _, d := range docs {
		if d.Mtknr == "" {
			continue // legacy pair-level rating, no longer used
		}
		out = append(out, &model.StudentConflictAcceptance{Ancode1: d.Ancode1, Ancode2: d.Ancode2, Mtknr: d.Mtknr})
	}
	return out, nil
}

// UpsertAcceptance stores (or replaces) a per-student acceptance by (ancode1, ancode2,
// mtknr).
func (db *DB) UpsertAcceptance(ctx context.Context, ancode1, ancode2 int, mtknr string) error {
	collection := db.getCollectionSemester(collectionExamConflictRatings)
	filter := bson.M{"ancode1": ancode1, "ancode2": ancode2, "mtknr": mtknr}
	doc := acceptanceDoc{Ancode1: ancode1, Ancode2: ancode2, Mtknr: mtknr}
	if _, err := collection.ReplaceOne(ctx, filter, doc, options.Replace().SetUpsert(true)); err != nil {
		log.Error().Err(err).Msg("cannot upsert conflict acceptance")
		return err
	}
	return nil
}

// DeleteAcceptance removes a per-student acceptance by (ancode1, ancode2, mtknr).
func (db *DB) DeleteAcceptance(ctx context.Context, ancode1, ancode2 int, mtknr string) (bool, error) {
	collection := db.getCollectionSemester(collectionExamConflictRatings)
	res, err := collection.DeleteOne(ctx, bson.M{"ancode1": ancode1, "ancode2": ancode2, "mtknr": mtknr})
	if err != nil {
		log.Error().Err(err).Msg("cannot delete conflict acceptance")
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
