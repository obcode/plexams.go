package db

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
)

// ReplaceRawCollection drops the named collection and inserts the given documents
// (used for the Primuss XLSX imports: studentregs_/exams_/count_/conflicts_<program>).
// Returns the number of inserted documents.
func (db *DB) ReplaceRawCollection(ctx context.Context, name string, docs []bson.M) (int, error) {
	coll := db.Client.Database(db.databaseName).Collection(name)
	if err := coll.Drop(ctx); err != nil {
		return 0, err
	}
	if len(docs) == 0 {
		return 0, nil
	}
	arr := make([]interface{}, len(docs))
	for i, d := range docs {
		arr[i] = d
	}
	res, err := coll.InsertMany(ctx, arr)
	if err != nil {
		return 0, err
	}
	return len(res.InsertedIDs), nil
}

// RawCollection returns all documents of a collection as bson.M (empty if missing).
func (db *DB) RawCollection(ctx context.Context, name string) ([]bson.M, error) {
	cur, err := db.Client.Database(db.databaseName).Collection(name).Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	docs := make([]bson.M, 0)
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	return docs, nil
}
