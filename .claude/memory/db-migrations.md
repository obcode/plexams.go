---
name: db-migrations
description: goose owns the PostgreSQL structure, semester.schema_version the data shape; PG.Migrate no longer migrates, it only guards against newer data
metadata:
  node_type: memory
  type: project
---

**Since the PostgreSQL cut-over (2026-08-04)** there are two separate versions, and they
are deliberately not merged ([db/migrate_pg.go](db/migrate_pg.go)):

- **goose** owns the *structure*: `db/migrations/*.sql`, embedded in the binary, applied
  by `MigratePG` at startup. A failure is **fatal** — a half-migrated schema does not match
  the queries compiled into the binary.
- **`semester.schema_version`** owns what a semester's rows *mean*.
  `CurrentSchemaVersion`/`MinSupportedSchemaVersion` live in [db/types.go](db/types.go).
  `PG.Migrate` ([db/semester_registry_pg.go](db/semester_registry_pg.go)) **no longer
  migrates anything**: the only data migration ever released renamed MongoDB collections,
  and in PostgreSQL every semester is created at `CurrentSchemaVersion`. What is left is
  the guard — data written by a newer binary is left alone, read-only semesters are
  skipped.

**Adding a data migration** therefore means putting a real step back into `PG.Migrate`
and bumping `CurrentSchemaVersion` together with the goose migration that changes the
shape callers see (see the comment on the constant). Write it idempotent: a crash between
the step and the version stamp re-runs it.

The MongoDB framework before it (`db/migrations.go`, `loadSemesterMeta` → `Migrate` →
`EnsureIndexes`) is deleted. Related: [[pg-first-boot]].
