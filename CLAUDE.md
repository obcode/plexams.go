# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

`plexams.go` is a GraphQL/REST server for planning university exams (Prüfungsplanung) at HM (Hochschule München, FK07). It imports exam and teacher data from the **ZPA** system and student registration/conflict data from **Primuss**, connects them, and helps schedule exams, assign rooms, and plan invigilations (Aufsichten). It is the backend for `plexams.gui` and is driven entirely through that GUI — there is no CLI anymore (the former Cobra `cmd/` was removed; the binary only starts the server). Much domain terminology and most comments/log messages are in German.

## Build, Test, Lint

```bash
go build -o plexams.go .         # build the binary
go test ./...                    # run all tests
go test ./plexams/ -run TestName # run a single test
go vet ./...
golangci-lint-v2 run             # the linter used in CI/pre-commit (note the -v2 binary name)
go generate ./...                # regenerate gqlgen code after editing *.graphqls (see main.go //go:generate)
sqlc generate                    # regenerate db/sqlc/ after editing db/queries/*.sql or a migration
```

**The `db/` integration tests need a PostgreSQL and skip themselves without one** — so
"green" and "never ran" look the same unless you say otherwise:

```bash
export PLEXAMS_TEST_PG_URI="postgres://plexams@127.0.0.1:5433/plexams?sslmode=disable"
export PLEXAMS_TEST_PG_REQUIRED=1   # turns a skip into a failure, as CI does
```

`internal/pgtest` gives each test its own database, created from a migrated template
named after the migrations' checksum — editing a migration produces a fresh template
rather than silently reusing a stale schema.

Pre-commit hooks (`.pre-commit-config.yaml`) run `gofmt -w`, `go vet`, `golangci-lint-v2`, and gitleaks. Run `pre-commit install` once.

## Running

`plexams.go` (no subcommands) starts the GraphQL/REST server: GraphQL playground at `/`, queries/mutations at `POST /query`, subscriptions over websocket, plus REST up/download routes on the same chi router (`/upload/...`, `/download/...`; default CORS/origin allows localhost:5173/8080/3000). Only three flags remain, parsed by the `bootstrap` package with the standard `flag` package: `-v/--verbose`, `--db-uri`, `--semester`.

A typical run requires PostgreSQL and config (see below). Everything that used to be a CLI command is now a GraphQL query/mutation/subscription or a REST endpoint, driven from `plexams.gui` — e.g. creating a semester (`createSemester` mutation), imports/uploads (ZPA/Primuss subscriptions + `/upload/...`), generation and validation (subscriptions), emails (send* subscriptions), and document downloads (`/download/pdf/{kind}`, `/download/csv/{kind}`, `/download/ics/{program}`, semester dump/dataset ZIP/CSV). The end-to-end planning workflow is documented in [README.md](README.md).

## Configuration

Config is loaded via **viper** from a single file `.plexams.yaml` (in `.` or `$HOME`) by the `bootstrap` package (`initConfig`); it may optionally pin a `semester` (e.g. `2026-SS`). There is **no** per-semester `<semester>.yaml` merge anymore — the per-semester config (`semesterConfig.*` incl. per-joint-program reserved times) lives in and is loaded from the database (table `semester_config_input`), edited through the GUI. Without a pinned semester the active one is auto-selected from the DB (switchable at runtime via the `setSemester` GUI mutation). Key config sections consumed in code: `db.uri` (there is no `db.database` any more — a semester is a `semester_id`, and `--semester` says which), `zpa.*` (baseurl, username, password, fk07programs, oldprograms), `smtp.*`, `planer.*` (the server-wide DEFAULT planner — the email sender identity; a semester overrides it in the DB table `semester_planer`, edited in the GUI), `operator.*` (local identity of the person running this instance — stamped onto the `mutation_log` for the audit "who did what"), `server.port`/`server.allowedorigins`/`server.production` (production turns off the GraphQL playground + introspection); `auth.*` (server deployment behind an auth proxy — see below); the per-semester `semesterConfig.*` (from/until/slots/forbiddenDays/emails/per-joint-program reserved times) comes from the DB (YAML still works as a fallback/seed); `scheduler.*` (server-only nightly auto-sync — see below). Secrets stay in the file, never in the DB.

### Nightly auto-sync (`scheduler.*`)

On the server an in-process goroutine (`plexams/scheduler`, started from `graph.StartServer`, cancelled on the shutdown signal) runs a daily job at `scheduler.time` (local, default 03:00) when `scheduler.enabled` is set. It calls `Plexams.RunScheduledSync` ([plexams/autosync.go](plexams/autosync.go)), which rebuilds a fresh ZPA client (`ResetZPA` — the long-lived client caches teachers/exams in memory), re-imports ZPA exams/teachers/invigilator-requirements + Anny bookings for the **active** semester via the existing import methods (holding the exclusive-op guard so it never collides with a manual transfer), diffs each against the previous state and mails the result: a change mail to `scheduler.changesrecipient` only when something differs, plus a heartbeat mail to `scheduler.debugrecipient` on every run. The diff detail is read back from the per-source `SyncLogEntry` records (`sync_log`); the Anny import now writes a full diff too (`plexams/anny/fetch.go`, reusing `zpaimport.DiffChanges` which is generic over the key type). The whole job can be triggered on demand from the GUI via the `triggerScheduledSync` subscription. The manual per-source GUI imports are unchanged.

### Auth / roles (server deployment)

For the server deployment plexams.go sits behind an auth proxy (Caddy `forward_auth` → `oauth2-proxy` doing OIDC against `sso.hm.edu`) that authenticates the user and injects a trusted identity header. The backend does **not** authenticate itself — `authMiddleware` ([graph/auth.go](graph/auth.go)) trusts `auth.header` (default `X-Remote-User`) and authorizes it against the global `users` table ([db/users_pg.go](db/users_pg.go), fail-closed allow-list), injecting a `*model.User` into the request context (`UserFromContext`). **Authorization is enforced in the backend** (single role per user, hierarchy `ADMIN` ⊇ `PLANER` ⊇ `VIEWER`: `VIEWER` = read-only, gated in `AroundOperations`; `PLANER` = full planning; `ADMIN` = also user administration, `setUser`/`removeUser` gated via `requireAdmin`); the GUI is never a security boundary. When `auth.enabled` is false (default → local dev) a full-access `ADMIN` dev user is injected so local operation is unchanged. The `mutation_log` "who" comes from the ctx principal on the server (`auditUser`), falling back to the local `operator.*` config. `auth.*` (incl. `auth.seedusers`) is kept strictly separate from the planner (the per-semester email sender identity). Ready-made deployment artifacts (docker-compose, Caddyfile + oauth2-proxy config, ACME/EAB TLS, config examples, smoke tests) live in the **private** `plexams.dev` repository under `deploy/` — this repository is public, and the deployment names a host that is not. A release here triggers the deploy there via `repository_dispatch` (`.github/workflows/docker.yml`).

## Architecture

Layers, each its own package, wired together in `bootstrap/` + `main.go`:

- **`bootstrap/`** — server entrypoint. `main.go` sets version/timezone and calls `bootstrap.Serve()`, which parses the three flags, loads config (`initConfig`), constructs the `*plexams.Plexams` instance (`newPlexams`) and starts the server. This replaced the former Cobra `cmd/` package (removed). Documents/exports that used to be CLI file writers are now REST download handlers; there is no code path that writes generated files to disk.
- **`plexams/`** — Business logic. The central `Plexams` struct ([plexams/plexams.go](plexams/plexams.go)) holds the semester string, a `*db.PG`, ZPA/email/planer config, the computed `*model.SemesterConfig`, and a room-info map. Almost all domain operations are methods on `*Plexams`, grouped by concern across many files (`exam.go`, `plan.go`, `rooms*.go`, `invigilation*.go`, `nta.go`, `constraints.go`, `email_*.go`, `validate_*.go`, `pdf*.go`, `csv.go`, `ics.go`, `primuss*.go`, `zpa*.go`). Exams are stored as absolute times (the slot/day ordinals were removed in the slotless refactor).
- **`db/`** — PostgreSQL persistence (pgx + sqlc). `PG` struct in [db/pg.go](db/pg.go), one `*_pg.go` file per data type mirroring the plexams files. **All semesters share one schema**; every semester-scoped table carries a `semester_id` column with a foreign key onto `semester`, and the client filters on the semester it is pointed at. Hand-written SQL in `db/queries/*.sql` → generated code in `db/sqlc/` (`sqlc generate`); schema in `db/migrations/*.sql` (goose), embedded in the binary and applied by the server at startup. Where nothing constrains or queries a field, it is stored as `jsonb` on the model type instead of being normalised.
- **`zpa/`** — HTTP client for the external ZPA REST API (teachers, exams, invigilator requirements, student regs, plan upload). The `Plexams.zpa.client` is created lazily via `SetZPA()`.
- **`graph/`** — GraphQL API (gqlgen). Resolvers delegate to `*Plexams`. Server bootstrap in [graph/server.go](graph/server.go).

### GraphQL / code generation

The GraphQL layer is generated by **gqlgen** (`gqlgen.yml`). Edit schema in `graph/*.graphqls`, then run `go generate ./...`. Generated/managed files — **do not edit by hand**:
- `graph/generated/generated.go`
- `graph/model/models_gen.go`
- `graph/*.resolvers.go` (resolver method stubs; you fill in the bodies, but gqlgen manages the signatures)

Hand-written model types live alongside generated ones in `graph/model/` (e.g. `exam.go`, `plan.go`) and are autobound. `resolver.go` holds the root `Resolver` (wraps `*Plexams`).

## Conventions

- **Logging:** `zerolog` (`github.com/rs/zerolog/log`) everywhere — `log.Error().Err(err).Msg(...)`. Only the server bootstrap (`bootstrap/`) uses `log.Fatal()` on unrecoverable startup errors.
- **Timezone:** `main.go` forces `time.Local = Europe/Berlin`. Slot/day calculations depend on this; preserve it.
- **Data direction:** `teachers` and `zpaexams` are imported from ZPA and must **not** be modified locally (see README). `zpaexamsToPlan`, `constraints`, connected/external exams are the locally-planned overlay.
- Versioning is injected via ldflags into `main.version/commit/date/builtBy` (see Dockerfile / `.goreleaser.yml`).
