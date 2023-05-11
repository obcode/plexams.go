package db

import (
	"context"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

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

	sort.Strings(programs)

	return programs, err
}
