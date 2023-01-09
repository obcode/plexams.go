package db

import (
	"context"

	"github.com/obcode/plexams.go/zpa"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func (db *DB) GetInvigilatorRequirements(ctx context.Context, teacherID int) (*zpa.SupervisorRequirements, error) {
	collection := db.getCollectionSemester(collectionInvigilatorRequirements)

	var req zpa.SupervisorRequirements

	filter := bson.D{{Key: "invigilatorid", Value: teacherID}}
	err := collection.FindOne(ctx, filter).Decode(&req)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		log.Error().Err(err).Int("invigilatorid", teacherID).Msg("cannot get requirements for inviglator")
		return nil, err
	}

	return &req, nil
}
