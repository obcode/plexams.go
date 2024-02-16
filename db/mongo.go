package db

import (
	"context"
	"strings"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

type DB struct {
	Client       *mongo.Client
	semester     string
	databaseName string
}

func NewDB(uri, semester string, dbName *string) (*DB, error) {
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}
	err = client.Ping(context.Background(), readpref.Primary())
	if err != nil {
		return nil, err
	}

	databaseName := strings.Replace(semester, " ", "-", 1)
	if dbName != nil {
		databaseName = *dbName
	}

	log.Debug().Str("database name", databaseName).Msg("using database")

	return &DB{
		Client:       client,
		semester:     semester,
		databaseName: databaseName,
	}, nil
}

func semesterName(semester string) string {
	return strings.Replace(semester, "-", " ", 1)
}

func (db *DB) SetSemester(semester string) error {
	db.semester = semester
	return nil
}
