package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
)

func (db *DB) AddNonZpaExam(ctx context.Context, exam *model.ZPAExam) error {
	_, err := db.Client.Database(db.databaseName).Collection(collectionNonZpaExams).InsertOne(ctx, exam)
	return err
}
