package db_test

import (
	"testing"

	"github.com/obcode/plexams.go/internal/pgtest"
)

// The server migrates on every start, so the second start of an unchanged binary
// runs the migrations against a database that already has them. That has to be a
// no-op rather than an error -- otherwise a plain restart would fail, and every
// container restart is one.
//
// It also covers the deployment case the migrations exist for: the release image
// carries no goose binary, so MigrateSchema is the only thing that ever applies
// them. If it could not run twice, it could not run in production at all.
func TestMigrateSchemaIsRepeatable(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	// pgtest built this database from the migrated template, so this is already
	// the "schema is current" case.
	if err := pg.MigrateSchema(ctx); err != nil {
		t.Fatalf("MigrateSchema on an already migrated database: %v", err)
	}
	if err := pg.MigrateSchema(ctx); err != nil {
		t.Fatalf("MigrateSchema, second run: %v", err)
	}

	// The connection still works afterwards: goose runs through a database/sql
	// handle wrapped around the pool, and closing that handle must not take the
	// pool with it.
	if _, err := pg.AllSemesterNames(ctx); err != nil {
		t.Errorf("the pool is unusable after migrating: %v", err)
	}
}
