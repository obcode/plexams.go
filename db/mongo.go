package db

import (
	"context"
	"strings"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

const (
	collectionNameAdditionalExams = "additional_exams"
	collectionNameConnectedExams  = "connected_exams"
)

type DB struct {
	Client   *mongo.Client
	semester string
}

func NewDB(uri, semester string) (*DB, error) {
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}
	err = client.Ping(context.Background(), readpref.Primary())
	if err != nil {
		return nil, err
	}

	return &DB{
		Client:   client,
		semester: semester,
	}, nil
}

func databaseName(semester string) string {
	return strings.Replace(semester, " ", "-", 1)
}

func semesterName(semester string) string {
	return strings.Replace(semester, "-", " ", 1)
}

func (db *DB) SetSemester(semester string) error {
	db.semester = semester
	return nil
}
