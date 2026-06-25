package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// StudyPrograms returns all study programs (Studiengänge). This list lives in
// the global "plexams" database and carries over between semesters.
func (db *DB) StudyPrograms(ctx context.Context) ([]*model.StudyProgram, error) {
	collection := db.Client.Database("plexams").Collection(collectionStudyPrograms)

	cur, err := collection.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "shortname", Value: 1}}))
	if err != nil {
		log.Error().Err(err).Msg("cannot find study programs")
		return nil, err
	}

	programs := make([]*model.StudyProgram, 0)
	if err := cur.All(ctx, &programs); err != nil {
		log.Error().Err(err).Msg("cannot decode study programs")
		return nil, err
	}
	return programs, nil
}

// StudyProgram returns one study program by its shortname, or nil when none.
func (db *DB) StudyProgram(ctx context.Context, shortname string) (*model.StudyProgram, error) {
	collection := db.Client.Database("plexams").Collection(collectionStudyPrograms)

	var program model.StudyProgram
	err := collection.FindOne(ctx, bson.M{"shortname": shortname}).Decode(&program)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		log.Error().Err(err).Str("shortname", shortname).Msg("cannot get study program")
		return nil, err
	}
	return &program, nil
}

// UpsertStudyProgram creates or replaces one study program (key: shortname).
func (db *DB) UpsertStudyProgram(ctx context.Context, program *model.StudyProgram) error {
	collection := db.Client.Database("plexams").Collection(collectionStudyPrograms)

	_, err := collection.ReplaceOne(ctx,
		bson.M{"shortname": program.Shortname},
		program,
		options.Replace().SetUpsert(true))
	if err != nil {
		log.Error().Err(err).Str("shortname", program.Shortname).Msg("cannot upsert study program")
		return err
	}
	return nil
}

// DeleteStudyProgram removes one study program. Returns false if there was none.
func (db *DB) DeleteStudyProgram(ctx context.Context, shortname string) (bool, error) {
	collection := db.Client.Database("plexams").Collection(collectionStudyPrograms)

	res, err := collection.DeleteOne(ctx, bson.M{"shortname": shortname})
	if err != nil {
		log.Error().Err(err).Str("shortname", shortname).Msg("cannot delete study program")
		return false, err
	}
	return res.DeletedCount > 0, nil
}
