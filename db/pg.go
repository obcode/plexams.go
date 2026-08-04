package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/rs/zerolog/log"
)

// PG is the PostgreSQL persistence layer.
//
// Unlike the Mongo DB struct there is no databaseName to repoint: a semester is a
// semesterID value carried into the queries, not a physical database. The field is
// still process-global (SwitchTo assigns it), so the concurrency semantics are
// unchanged -- de-globalising Plexams is a separate project.
type PG struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
	// uri is kept for DBHost, which shows the host in the admin view.
	uri string
	// semesterID is the semester being planned, "2026-WS" -- the value every
	// semester-scoped query filters on. There is no second field for the logical
	// semester ZPA knows ("2026 WS"): semesterName derives it. The two could only
	// ever differ for a test clone, and those are gone.
	semesterID string
}

// txKey carries a pgx.Tx through the context so that every db method picks up an
// enclosing transaction automatically. This is the whole reason db methods call
// db.q(ctx) instead of holding a *sqlc.Queries: without it every write path would
// need a second, transaction-aware variant of itself.
type txKey struct{}

// NewPG connects and verifies the connection.
//
// The session time zone is pinned to Europe/Berlin. Slot and day arithmetic all
// over plexams depends on time.Local being Europe/Berlin (set in main.go), and
// several maps are keyed by time.Time -- where a value carrying a UTC location is
// a *different key* for the same instant. See TestTimestamptzKeepsLocation.
func NewPG(ctx context.Context, uri, semesterID string) (*PG, error) {
	cfg, err := pgxpool.ParseConfig(uri)
	if err != nil {
		return nil, fmt.Errorf("cannot parse postgres uri: %w", err)
	}
	cfg.ConnConfig.RuntimeParams["timezone"] = "Europe/Berlin"

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("cannot reach postgres: %w", err)
	}

	log.Debug().Str("semesterID", semesterID).Msg("connected to postgres")

	return &PG{
		pool:       pool,
		queries:    sqlc.New(pool),
		uri:        uri,
		semesterID: semesterID,
	}, nil
}

func (db *PG) Close() {
	db.pool.Close()
}

// q returns the query set to use: the one bound to the transaction in ctx if there
// is one, otherwise the pool-backed one. Every db method must go through this --
// using db.queries directly inside a transaction would silently run outside it.
func (db *PG) q(ctx context.Context) *sqlc.Queries {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return db.queries.WithTx(tx)
	}
	return db.queries
}

// InTransaction runs fn inside a transaction, committing on success and rolling
// back on error or panic. fn MUST use the context it is handed -- that context
// carries the session, and an operation using the outer one runs outside the
// transaction and is not rolled back.
//
// Unlike the Mongo predecessor there is no non-atomic fallback: PostgreSQL needs
// no replica set, so a partial write is a bug rather than a deployment topology.
func (db *PG) InTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("cannot begin transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			// Rollback after a successful commit is a no-op error we do not care
			// about; a rollback triggered by a panic must not mask the panic.
			_ = tx.Rollback(context.WithoutCancel(ctx))
		}
	}()

	if err := fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("cannot commit transaction: %w", err)
	}
	committed = true

	return nil
}
