package db

import (
	"context"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

type DB struct {
	Client       *mongo.Client
	uri          string
	semester     string
	databaseName string
	// todosMu serializes the drop+insert in CacheInvigilatorTodos so concurrent
	// callers (e.g. parallel validation subscriptions) cannot interleave their
	// drops and inserts and leave more than one todos document behind.
	todosMu sync.Mutex
}

func NewDB(uri, semester string, dbName *string) (*DB, error) {
	// MongoDB stores all datetimes as UTC. Decode them back into the local
	// timezone (Europe/Berlin, set in main.go via time.Local) so that the rest
	// of plexams.go works with local time everywhere, matching the local times
	// given in the semester YAML config.
	client, err := mongo.Connect(context.Background(),
		options.Client().
			ApplyURI(uri).
			SetBSONOptions(&options.BSONOptions{UseLocalTimeZone: true}))
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
		uri:          uri,
		semester:     semester,
		databaseName: databaseName,
	}, nil
}

func semesterName(semester string) string {
	return strings.Replace(semester, "-", " ", 1)
}

// MongoHost returns the host:port the client is connected to, with any
// credentials (user:pass@) and path/query stripped, so it is safe to display.
func (db *DB) MongoHost() string {
	s := db.uri
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if at := strings.LastIndex(s, "@"); at >= 0 {
		s = s[at+1:]
	}
	if i := strings.IndexAny(s, "/?"); i >= 0 {
		s = s[:i]
	}
	return s
}
