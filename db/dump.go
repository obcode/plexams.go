package db

import (
	"context"
	"sort"

	"go.mongodb.org/mongo-driver/bson"
)

// AllCollectionNames returns the names of all collections in the current
// (per-semester) database, sorted. Used by the semester dump/restore.
func (db *DB) AllCollectionNames(ctx context.Context) ([]string, error) {
	names, err := db.Client.Database(db.databaseName).ListCollectionNames(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}

// CountCollection returns the number of documents in a collection (0 if missing).
func (db *DB) CountCollection(ctx context.Context, name string) (int64, error) {
	return db.Client.Database(db.databaseName).Collection(name).CountDocuments(ctx, bson.M{})
}

// InsertRawDocs inserts the given documents into a collection without dropping it
// first (used to merge external-exam plan entries back into the shared plan
// collection). Returns the number of inserted documents.
func (db *DB) InsertRawDocs(ctx context.Context, name string, docs []bson.M) (int, error) {
	if len(docs) == 0 {
		return 0, nil
	}
	arr := make([]interface{}, len(docs))
	for i, d := range docs {
		arr[i] = d
	}
	res, err := db.Client.Database(db.databaseName).Collection(name).InsertMany(ctx, arr)
	if err != nil {
		return 0, err
	}
	return len(res.InsertedIDs), nil
}

// DeleteDocsByAncodes deletes every document of a collection whose "ancode" is in
// the given set (used to clear the external-exam entries of the shared plan
// collection before re-inserting them, without touching the regular plan).
func (db *DB) DeleteDocsByAncodes(ctx context.Context, name string, ancodes []int) (int64, error) {
	if len(ancodes) == 0 {
		return 0, nil
	}
	res, err := db.Client.Database(db.databaseName).Collection(name).
		DeleteMany(ctx, bson.M{"ancode": bson.M{"$in": ancodes}})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}
