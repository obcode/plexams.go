package db

import (
	"context"
	"sort"

	"github.com/obcode/plexams.go/graph/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func (client *Client) AllSemesterNames() ([]*model.Semester, error) {
	dbs, err := client.Client.ListDatabaseNames(context.Background(),
		bson.D{primitive.E{
			Key: "name",
			Value: bson.D{
				primitive.E{Key: "$regex",
					Value: primitive.Regex{Pattern: "[0-9]{4}-[WS]S"},
				},
			},
		}})
	if err != nil {
		return nil, err
	}

	sort.Strings(dbs)

	semester := make([]*model.Semester, len(dbs))
	n := len(dbs)
	for i, dbName := range dbs {
		semester[n-i-1] = &model.Semester{
			ID: dbName,
		}
	}

	return semester, nil
}
