// Package mongotest provides an ephemeral MongoDB for integration tests. It returns a
// ready-to-use *db.DB backed by a throwaway database that is dropped when the test ends.
//
// The MongoDB is obtained in one of two ways, in order:
//
//   - if PLEXAMS_TEST_MONGO_URI is set, that server is used (a uniquely named database is
//     created on it and dropped afterwards) — handy for a locally running MongoDB or a CI
//     service container;
//   - otherwise a MongoDB testcontainer is started (needs a running Docker daemon).
//
// If neither is available the test is skipped, so `go test ./...` stays green on machines
// without Docker or a test MongoDB.
package mongotest

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/obcode/plexams.go/db"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// NewDB returns a *db.DB pointing at a fresh, uniquely named database on an ephemeral
// MongoDB. The database (and, for a container, the container) is cleaned up via
// t.Cleanup. The test is skipped when no MongoDB can be provided.
func NewDB(t *testing.T) *db.DB {
	t.Helper()
	uri := mongoURI(t)

	name := "plexams_test_" + primitive.NewObjectID().Hex()
	dbClient, err := db.NewDB(uri, "test semester", &name)
	if err != nil {
		t.Fatalf("cannot connect to test MongoDB: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = dbClient.Client.Database(name).Drop(ctx)
		_ = dbClient.Client.Disconnect(ctx)
	})
	return dbClient
}

// mongoURI returns a connection string to an ephemeral MongoDB, or skips the test when
// none can be provided.
func mongoURI(t *testing.T) string {
	t.Helper()
	if uri := os.Getenv("PLEXAMS_TEST_MONGO_URI"); uri != "" {
		return uri
	}

	ctx := context.Background()
	ctr, err := mongodb.Run(ctx, "mongo:7")
	if err != nil {
		t.Skipf("no test MongoDB: set PLEXAMS_TEST_MONGO_URI or start Docker for testcontainers (%v)", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = ctr.Terminate(ctx)
	})
	uri, err := ctr.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("cannot get testcontainer connection string: %v", err)
	}
	return uri
}
