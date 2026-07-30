package db_test

import (
	"context"
	"sync"
	"testing"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/mongotest"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func todosCollection(d *db.DB) *mongo.Collection {
	return d.Client.Database(d.DatabaseName()).Collection("invigilator_todos")
}

// TestCacheInvigilatorTodosIsAtomic covers the race the removed mutex used to guard:
// concurrent cache writes must leave exactly one document behind.
func TestCacheInvigilatorTodosIsAtomic(t *testing.T) {
	d := mongotest.NewDB(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			if err := d.CacheInvigilatorTodos(ctx,
				&model.InvigilationTodos{InvigilatorCount: n}); err != nil {
				t.Errorf("CacheInvigilatorTodos: %v", err)
			}
		}(i)
	}
	wg.Wait()

	n, err := todosCollection(d).CountDocuments(ctx, bson.M{})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("got %d todos documents after concurrent writes, want exactly 1", n)
	}

	todos, err := d.GetInvigilationTodos(ctx)
	if err != nil || todos == nil {
		t.Fatalf("GetInvigilationTodos: %v (todos=%v)", err, todos)
	}
}

// TestCacheInvigilatorTodosHealsLegacyDocuments verifies that documents written
// before the fixed _id (and leftovers of an earlier interleaved write) are removed,
// and that they are still readable until then.
func TestCacheInvigilatorTodosHealsLegacyDocuments(t *testing.T) {
	d := mongotest.NewDB(t)
	ctx := context.Background()

	legacy := []any{
		model.InvigilationTodos{InvigilatorCount: 1},
		model.InvigilationTodos{InvigilatorCount: 2},
	}
	if _, err := todosCollection(d).InsertMany(ctx, legacy); err != nil {
		t.Fatal(err)
	}

	// Readable before any re-cache, even with the old ObjectID _ids.
	if todos, err := d.GetInvigilationTodos(ctx); err != nil || todos == nil {
		t.Fatalf("legacy read: %v (todos=%v)", err, todos)
	}

	if err := d.CacheInvigilatorTodos(ctx, &model.InvigilationTodos{InvigilatorCount: 42}); err != nil {
		t.Fatal(err)
	}

	n, err := todosCollection(d).CountDocuments(ctx, bson.M{})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("got %d todos documents, want 1 after healing", n)
	}
	todos, err := d.GetInvigilationTodos(ctx)
	if err != nil || todos == nil {
		t.Fatalf("GetInvigilationTodos: %v", err)
	}
	if todos.InvigilatorCount != 42 {
		t.Errorf("got InvigilatorCount %d, want 42", todos.InvigilatorCount)
	}
}
