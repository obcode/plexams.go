package db

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func (client *Client) GetSemester() ([]string, error) {
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
	return dbs, nil
}
