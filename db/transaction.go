package db

import (
	"context"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// detectTransactionSupport reports whether the server can run multi-document
// transactions, which requires a replica set (or mongos). A standalone mongod cannot,
// and that is a supported deployment here: production ran standalone until the replica
// set rollout, and developer machines may still.
func detectTransactionSupport(ctx context.Context, client *mongo.Client) bool {
	var hello struct {
		SetName string `bson:"setName"`
		Msg     string `bson:"msg"`
	}
	err := client.Database("admin").
		RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello)
	if err != nil {
		log.Warn().Err(err).Msg("cannot determine whether transactions are available, assuming not")
		return false
	}
	// msg == "isdbgrid" identifies a mongos, which supports transactions too.
	return hello.SetName != "" || hello.Msg == "isdbgrid"
}

// SupportsTransactions reports whether writes spanning several documents are atomic.
func (db *DB) SupportsTransactions() bool {
	return db.supportsTransactions
}

// InTransaction runs fn atomically where the deployment allows it, for callers outside
// this package that need several database calls to succeed or fail together.
//
// Two rules for fn:
//   - every database call inside it MUST use the context fn receives, which carries the
//     session; a call using the outer context silently runs outside the transaction;
//   - fn may be RETRIED on a transient error, so it must not accumulate side effects
//     outside the database — build any result fresh on each invocation.
func (db *DB) InTransaction(ctx context.Context, fn func(context.Context) error) error {
	return db.withTransaction(ctx, fn)
}

// withTransaction runs fn atomically when the deployment allows it, and plainly when it
// does not. fn MUST use the context it is handed for every database call — that context
// carries the session, and an operation using the outer context would silently run
// outside the transaction.
//
// Falling back instead of failing is deliberate: a standalone mongod must keep working.
// The caller therefore gets the same behaviour as before, just without atomicity.
func (db *DB) withTransaction(ctx context.Context, fn func(context.Context) error) error {
	if !db.supportsTransactions {
		return fn(ctx)
	}

	session, err := db.Client.StartSession()
	if err != nil {
		log.Error().Err(err).Msg("cannot start session, running without a transaction")
		return fn(ctx)
	}
	defer session.EndSession(ctx)

	_, err = session.WithTransaction(ctx,
		func(sessionCtx mongo.SessionContext) (interface{}, error) {
			return nil, fn(sessionCtx)
		})
	return err
}
