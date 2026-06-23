package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// InvigilatorConstraints returns all per-invigilator constraints stored in the
// DB (managed via the GUI), separate from the ZPA-sourced requirements.
func (db *DB) InvigilatorConstraints(ctx context.Context) ([]*model.InvigilatorConstraints, error) {
	collection := db.getCollectionSemester(collectionInvigilatorConstraints)

	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Msg("cannot find invigilator constraints")
		return nil, err
	}

	constraints := make([]*model.InvigilatorConstraints, 0)
	if err := cur.All(ctx, &constraints); err != nil {
		log.Error().Err(err).Msg("cannot decode invigilator constraints")
		return nil, err
	}

	return constraints, nil
}

// InvigilatorConstraintsForTeacher returns the constraints of one invigilator, or
// nil if none are stored.
func (db *DB) InvigilatorConstraintsForTeacher(ctx context.Context, teacherID int) (*model.InvigilatorConstraints, error) {
	collection := db.getCollectionSemester(collectionInvigilatorConstraints)

	var constraints model.InvigilatorConstraints
	err := collection.FindOne(ctx, bson.M{"teacherid": teacherID}).Decode(&constraints)
	if err != nil {
		return nil, nil //nolint:nilerr // no document for the teacher is not an error
	}
	return &constraints, nil
}

// UpsertInvigilatorConstraints creates or replaces the whole constraints record
// of one invigilator (key: teacherID).
func (db *DB) UpsertInvigilatorConstraints(ctx context.Context, constraints *model.InvigilatorConstraints) error {
	collection := db.getCollectionSemester(collectionInvigilatorConstraints)

	_, err := collection.ReplaceOne(ctx,
		bson.M{"teacherid": constraints.TeacherID},
		constraints,
		options.Replace().SetUpsert(true))
	if err != nil {
		log.Error().Err(err).Int("teacherID", constraints.TeacherID).Msg("cannot upsert invigilator constraints")
		return err
	}
	return nil
}

// DeleteInvigilatorConstraints removes the constraints record of one invigilator.
// Returns false if there was none.
func (db *DB) DeleteInvigilatorConstraints(ctx context.Context, teacherID int) (bool, error) {
	collection := db.getCollectionSemester(collectionInvigilatorConstraints)

	res, err := collection.DeleteOne(ctx, bson.M{"teacherid": teacherID})
	if err != nil {
		log.Error().Err(err).Int("teacherID", teacherID).Msg("cannot delete invigilator constraints")
		return false, err
	}
	return res.DeletedCount > 0, nil
}
