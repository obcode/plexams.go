package db

import (
	"context"
	"embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// Migrations are embedded so a release carries its own schema: the binary and the
// DDL it expects can never drift apart across a deploy.
//
// goose owns the *structure*. It is deliberately NOT merged with the existing
// per-semester SchemaVersion (db/migrations.go), which versions the *shape of the
// data* inside a workspace -- two different questions with two different lifetimes.
//
//go:embed migrations/*.sql
var pgMigrationsFS embed.FS

// MigratePG brings the database up to the schema this binary was built with.
//
// Unlike EnsureIndexes this is fatal on failure: a half-migrated schema means the
// queries compiled into this binary do not match the database, and every later
// error would be a confusing symptom of that one cause.
func MigratePG(ctx context.Context, pool *pgxpool.Pool) error {
	goose.SetBaseFS(pgMigrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("cannot set goose dialect: %w", err)
	}

	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close() //nolint:errcheck

	if err := goose.UpContext(ctx, sqlDB, "migrations"); err != nil {
		return fmt.Errorf("cannot migrate database: %w", err)
	}

	return nil
}
