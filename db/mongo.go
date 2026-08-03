package db

import (
	"context"
	"strings"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// defaultGlobalDatabase holds the master data that carries over between semesters
// (rooms, NTAs, study programs, users, templates).
const defaultGlobalDatabase = "plexams"

type DB struct {
	Client   *mongo.Client
	uri      string
	semester string
	// databaseName is the current semester's database; globalDatabaseName holds the
	// cross-semester master data. Both are names, not handles, because the semester
	// one is repointed at runtime by SwitchTo.
	databaseName       string
	globalDatabaseName string
	// supportsTransactions is false against a standalone mongod, which cannot run
	// multi-document transactions. Writes then fall back to being non-atomic.
	supportsTransactions bool
}

// globalDatabase is the cross-semester master-data database. Always go through this
// instead of naming it literally, so tests can isolate it.
func (db *DB) globalDatabase() *mongo.Database {
	return db.Client.Database(db.globalDatabaseName)
}

// WithGlobalDatabase overrides the name of the global master-data database and returns
// db. Only meant for tests: without it they write their fixtures into the real master
// data, which is shared by every semester.
func (db *DB) WithGlobalDatabase(name string) *DB {
	db.globalDatabaseName = name
	return db
}

// GlobalDatabaseName returns the name of the master-data database.
func (db *DB) GlobalDatabaseName() string {
	return db.globalDatabaseName
}

// decorateInvigilation wraps the persisted Starttime in the Slot the API exposes
// (the absolute start time is the sole coordinate).
func (db *DB) decorateInvigilation(inv *model.Invigilation) {
	if inv == nil || inv.Starttime == nil {
		return
	}
	inv.Slot = &model.Slot{Starttime: *inv.Starttime}
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

	supportsTransactions := detectTransactionSupport(context.Background(), client)
	if !supportsTransactions {
		log.Warn().Msg("MongoDB is not a replica set: multi-document writes are not atomic")
	}

	return &DB{
		Client:               client,
		uri:                  uri,
		semester:             semester,
		databaseName:         databaseName,
		globalDatabaseName:   defaultGlobalDatabase,
		supportsTransactions: supportsTransactions,
	}, nil
}

func semesterName(semester string) string {
	return strings.Replace(semester, "-", " ", 1)
}

// MongoHost returns the host:port the client is connected to, with any
// credentials (user:pass@) and path/query stripped, so it is safe to display.
//
// Shares its implementation with PG.DBHost, its successor: both feed the same
// admin view, and two copies of a credential-stripping rule is one too many.
func (db *DB) MongoHost() string {
	return hostOf(db.uri)
}
