package db

import (
	"context"
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type PrimussType string

const (
	StudentRegs PrimussType = "studentregs"
	Exams       PrimussType = "exams"
	Counts      PrimussType = "count"
	Conflicts   PrimussType = "conflicts"
)

func (db *DB) getCollection(program string, primussType PrimussType) *mongo.Collection {
	return db.Client.Database(databaseName(db.semester)).Collection(fmt.Sprintf("%s_%s", primussType, program))
}

func (db *DB) GetPrograms(ctx context.Context) ([]string, error) {
	collectionNames, err := db.Client.Database(databaseName(db.semester)).ListCollectionNames(ctx,
		bson.D{primitive.E{
			Key: "name",
			Value: bson.D{
				primitive.E{Key: "$regex",
					Value: primitive.Regex{Pattern: "exams_"},
				},
			},
		}})

	programs := make([]string, 0)

	for _, name := range collectionNames {
		programs = append(programs, strings.Replace(name, "exams_", "", 1))
	}

	return programs, err
}
