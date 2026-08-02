---
name: pg-dev-setup
description: "PostgreSQL lokal betreiben ohne Docker, sqlc/goose als Release-Binaries, und die OOM-Fallen im DevContainer"
metadata:
  node_type: memory
  type: reference
  originSessionId: aa4db750-7c9f-46e3-9217-79bb0789c0b8
  modified: 2026-08-02T21:19:42.954Z
---

Für die PostgreSQL-Migration ([[postgres-migration]]). Der DevContainer hat **kein Docker**
— dieselbe Lage wie beim Standalone-`mongod` in [[mongotest-without-docker]]. Solange
`plexams.dev` keinen Postgres-Service mitbringt, läuft er direkt im Container.

**Server:** PostgreSQL 17 aus dem PGDG-Repo (Debians eigene Quelle hat nur 15):

```
sudo install -d /usr/share/postgresql-common/pgdg
sudo curl -sS -o /usr/share/postgresql-common/pgdg/apt.postgresql.org.asc --fail \
     https://www.postgresql.org/media/keys/ACCC4CF8.asc
echo "deb [signed-by=/usr/share/postgresql-common/pgdg/apt.postgresql.org.asc] \
      https://apt.postgresql.org/pub/repos/apt bookworm-pgdg main" \
  | sudo tee /etc/apt/sources.list.d/pgdg.list
sudo apt-get update && sudo apt-get install -y postgresql-17
```

Wegwerf-Instanz auf **:5433** (nicht 5432, damit ein späterer Compose-Service nicht
kollidiert). Startskript liegt versioniert in
**`/workspace/plexams.dev/scripts/pg-dev.sh`**:

```
scripts/pg-dev.sh start|stop|status|reset|psql
```

Das Cluster liegt unter `~/.local/share/plexams-pgdev` (überschreibbar via
`PLEXAMS_PGDEV_HOME`), also außerhalb des Repos — es ist Maschinenzustand, kein Quelltext.
**`reset`** legt die Datenbank neu an; das braucht man, solange Migrationen noch editiert
werden, weil `goose down` dann den *neuen* Down-Abschnitt gegen das *alte* Schema fährt und
scheitert.

```
PLEXAMS_TEST_PG_URI="postgres://plexams@127.0.0.1:5433/plexams?sslmode=disable"
PLEXAMS_TEST_PG_REQUIRED=1   # macht aus "übersprungen" ein "fehlgeschlagen"
```

**Locale bewusst `C.UTF-8`** beim `initdb`: Byte-Ordnung wie Mongos Binärvergleich, damit
die Migration nichts still umsortiert. Wo menschenfreundliche Sortierung gewollt ist,
explizit `COLLATE` anfordern.

## Die drei OOM-Fallen

Der Container hat 7,7 GiB, wovon `gopls` allein ~800 MB hält. Drei Dinge werden deshalb
**abgeschossen (Signal 9)**, und zwei davon sehen wie Erfolg aus:

1. **`go install` von sqlc scheitert** (zieht den TiDB-Parser). Stattdessen das
   Release-Binary: `github.com/sqlc-dev/sqlc/releases/.../sqlc_<v>_linux_arm64.tar.gz`.
   Gilt auch für das Container-Image und CI — sqlc dort nie bauen.
2. **`go install` von goose scheitert** (modernc.org/sqlite). Ebenfalls Release-Binary:
   `github.com/pressly/goose/releases/download/v<v>/goose_linux_arm64`. Die *Bibliothek*
   `github.com/pressly/goose/v3` ist dagegen leichtgewichtig und normal per `go get`.
3. **`golangci-lint-v2 run` wird ohne Ausgabe gekillt** — und eine leere Ausgabe liest
   sich wie „0 issues". **Immer den Exit-Code prüfen.** Es läuft durch mit:
   `GOGC=40 GOMAXPROCS=1 golangci-lint-v2 run`. Dieselben Variablen vor `git commit`
   setzen, sonst stirbt der pre-commit-Hook mit `exit code -9` — der Commit findet dann
   **nicht** statt, ein nachfolgendes `git push` schiebt aber klaglos den alten Stand.

## Werkzeuge

`sqlc generate` liest `sqlc.yaml` im Repo-Wurzelverzeichnis (Schema aus `db/migrations`,
Queries aus `db/queries`, Ausgabe nach `db/sqlc`). goose-Migrationen laufen produktiv
eingebettet (`db/migrate_pg.go`, `go:embed`); von Hand:
`goose -dir db/migrations postgres "$PLEXAMS_TEST_PG_URI" up|down-to 0|status`.
