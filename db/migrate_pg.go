package db

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// Migrations are embedded so a release carries its own schema: the binary and the
// DDL it expects can never drift apart across a deploy.
//
// goose owns the *structure*. It is deliberately NOT merged with the existing
// per-semester SchemaVersion (db/migrations.go), which versions the *shape of the
// data* inside a semester -- two different questions with two different lifetimes.
//
//go:embed migrations/*.sql
var pgMigrationsFS embed.FS

// MigrationsChecksum identifies the schema this binary carries. Two binaries with
// the same checksum expect the same tables.
//
// Used by the test harness to name its template database, so that editing a
// migration produces a fresh template instead of silently reusing a stale one.
func MigrationsChecksum() (string, error) {
	entries, err := fs.ReadDir(pgMigrationsFS, "migrations")
	if err != nil {
		return "", fmt.Errorf("cannot read migrations: %w", err)
	}

	sum := sha256.New()
	for _, entry := range entries {
		content, err := fs.ReadFile(pgMigrationsFS, "migrations/"+entry.Name())
		if err != nil {
			return "", fmt.Errorf("cannot read %s: %w", entry.Name(), err)
		}
		// The name matters as much as the content: a rename reorders the run.
		sum.Write([]byte(entry.Name()))
		sum.Write(content)
	}

	return hex.EncodeToString(sum.Sum(nil))[:12], nil
}

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

// MigrateSchema brings the connected database up to the schema this binary
// carries. It is what the server calls at startup.
//
// The migrations travel inside the binary for exactly this reason: the release
// image is alpine with nothing but the executable in it -- no goose, no psql --
// so nobody else *can* run them. Whoever deploys a new tag applies its schema by
// starting it.
//
// Repeating it is a no-op: goose applies only what is missing.
func (db *PG) MigrateSchema(ctx context.Context) error {
	return MigratePG(ctx, db.pool)
}
