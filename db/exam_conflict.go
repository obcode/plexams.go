package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// decisionDoc is a stored explicit per-student conflict decision. Key: (ancode1,
// ancode2, mtknr); Decision is ACCEPT or VETO.
type decisionDoc struct {
	Ancode1  int    `bson:"ancode1"`
	Ancode2  int    `bson:"ancode2"`
	Mtknr    string `bson:"mtknr"`
	Decision string `bson:"decision"`
}

// StudentConflictDecisions returns all stored explicit per-student decisions.
func (db *DB) StudentConflictDecisions(ctx context.Context) ([]*model.StudentConflictDecision, error) {
	collection := db.getCollectionSemester(collectionExamConflictRatings)
	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Msg("cannot get conflict decisions")
		return nil, err
	}
	var docs []decisionDoc
	if err := cur.All(ctx, &docs); err != nil {
		log.Error().Err(err).Msg("cannot decode conflict decisions")
		return nil, err
	}
	out := make([]*model.StudentConflictDecision, 0, len(docs))
	for _, d := range docs {
		if d.Mtknr == "" || d.Decision == "" {
			continue // legacy pair-level rating, no longer used
		}
		out = append(out, &model.StudentConflictDecision{
			Ancode1: d.Ancode1, Ancode2: d.Ancode2, Mtknr: d.Mtknr, Decision: model.ConflictDecision(d.Decision),
		})
	}
	return out, nil
}

// UpsertDecision stores (or replaces) an explicit decision by (ancode1, ancode2, mtknr).
func (db *DB) UpsertDecision(ctx context.Context, ancode1, ancode2 int, mtknr, decision string) error {
	collection := db.getCollectionSemester(collectionExamConflictRatings)
	filter := bson.M{"ancode1": ancode1, "ancode2": ancode2, "mtknr": mtknr}
	doc := decisionDoc{Ancode1: ancode1, Ancode2: ancode2, Mtknr: mtknr, Decision: decision}
	if _, err := collection.ReplaceOne(ctx, filter, doc, options.Replace().SetUpsert(true)); err != nil {
		log.Error().Err(err).Msg("cannot upsert conflict decision")
		return err
	}
	return nil
}

// DeleteDecision removes an explicit decision by (ancode1, ancode2, mtknr).
func (db *DB) DeleteDecision(ctx context.Context, ancode1, ancode2 int, mtknr string) (bool, error) {
	collection := db.getCollectionSemester(collectionExamConflictRatings)
	res, err := collection.DeleteOne(ctx, bson.M{"ancode1": ancode1, "ancode2": ancode2, "mtknr": mtknr})
	if err != nil {
		log.Error().Err(err).Msg("cannot delete conflict decision")
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
