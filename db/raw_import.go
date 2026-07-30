package db

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
)

// ReplaceRawCollection empties the named collection and inserts the given documents
// (used for the Primuss XLSX imports: studentregs_/exams_/count_/conflicts_<program>).
// Returns the number of inserted documents.
//
// It clears with DeleteMany rather than dropping, so the indexes EnsureIndexes created
// survive a re-import instead of only coming back on the next start. Use
// DropPrimussData to remove the collections themselves.
func (db *DB) ReplaceRawCollection(ctx context.Context, name string, docs []map[string]any) (int, error) {
	coll := db.Client.Database(db.databaseName).Collection(name)
	if _, err := coll.DeleteMany(ctx, bson.M{}); err != nil {
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

// RawCollection returns all documents of a collection as plain maps (empty if missing).
func (db *DB) RawCollection(ctx context.Context, name string) ([]map[string]any, error) {
	cur, err := db.Client.Database(db.databaseName).Collection(name).Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	docs := make([]map[string]any, 0)
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	return docs, nil
}
