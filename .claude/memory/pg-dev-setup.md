---
name: pg-dev-setup
description: "PostgreSQL lokal betreiben ohne Docker, sqlc/goose als Release-Binaries, und die OOM-Fallen im DevContainer"
metadata:
  node_type: memory
  type: reference
  originSessionId: aa4db750-7c9f-46e3-9217-79bb0789c0b8
  modified: 2026-08-03T16:13:55.014Z
---

Für die PostgreSQL-Migration ([[postgres-migration]]). Der DevContainer hat **kein Docker**
— dieselbe Lage wie beim Standalone-`mongod` in [[mongotest-without-docker]]. Der Server
läuft deshalb direkt im Container, nicht als Compose-Dienst.

**Server:** PostgreSQL 18 aus dem PGDG-Repo (Debians eigene Quelle hat nur 15).
**Seit 2026-08-03 liegt er samt `sqlc` und `goose` im `plexams.dev`-Image** (`PG_MAJOR`,
`SQLC_VERSION`, `GOOSE_VERSION` im Dockerfile) — von Hand nachinstallieren muss man
nichts mehr. Vorher steckten alle drei nur im laufenden Container und waren nach jedem
Rebuild weg.

Zwei Dinge, die beim Backen aufgefallen sind und bei einer Wiederholung Zeit sparen:

- Das Paket legt einen Default-Cluster an, den hier niemand braucht. Er wird **nach**
  der Installation per `pg_dropcluster --stop 18 main` entfernt, nicht vorab per
  `create_main_cluster = false`: diese Datei geht durch `ucf`, und ob eine vorab
  hingelegte Fassung überlebt, hängt am Debconf-Frontend. Löschen ist deterministisch.
- `/shared/bin` steht im `PATH` **vor** `/usr/local/bin`. Eine dort von Hand abgelegte
  `sqlc`/`goose`-Kopie überdeckt die Image-Version still und driftet irgendwann von
  `ci.yml` weg. `post-create.sh` räumt beide auf, wenn das Image sie mitbringt.

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

**Major-Version 18** (Stand 2026-08-02 aktuell, 18.4). Das Schema braucht nichts oberhalb
von 15 — `unique nulls not distinct` ist das Anspruchsvollste — der Wechsel von der zuerst
ungeprüft gesetzten 17 kostete daher nichts und wurde im Greenfield gemacht statt später als
Major-Upgrade im Betrieb. `needs_request` bleibt bewusst `stored`: PG 18 könnte die Spalte
`virtual` berechnen, aber virtuelle Spalten sind dort **nicht indizierbar**, und bei 37
Räumen ist der Speichergewinn null. **Ein Cluster gehört zu seiner Major-Version** —
`pg-dev.sh` prüft `PG_VERSION` und sagt beim Wechsel, dass das Verzeichnis wegzuwerfen ist,
statt mit einer irreführenden Meldung zu scheitern.

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

## Die Zeitzonen-Falle im CI

**Der GitHub-Runner läuft in UTC, der DevContainer in Europe/Berlin.** `main.go:22`
setzt `time.Local` zur Laufzeit auf Europe/Berlin — Tests gehen aber nicht durch
`main` und erben die Zone des Hosts. Ergebnis: `TestTimestamptzKeepsLocation` und
`TestSeparatelyLoadedLocationIsADifferentMapKey` waren lokal grün und im CI seit
ihrer Entstehung rot (`offset = +0 s, want +7200 s`).

Behoben mit einem `TestMain` in `db/timezone_test.go`, das dasselbe tut wie
`main.go`. Bewusst **nicht** über `TZ` im Workflow: so bleibt die Suite auch auf
einem Rechner korrekt, der nicht auf deutscher Zeit steht. `package db` und
`package db_test` landen im selben Testbinary, ein `TestMain` deckt beide ab.

**Reproduzieren:** `TZ=UTC go test ./db/` — das sind die Runner-Bedingungen.
Bei jedem neuen zeitabhängigen Test einmal so laufen lassen.

**Und: nach dem Pushen `gh run list --branch pg` schauen.** Lokal grün heißt hier
nachweislich nicht CI-grün.

## Werkzeuge

`sqlc generate` liest `sqlc.yaml` im Repo-Wurzelverzeichnis (Schema aus `db/migrations`,
Queries aus `db/queries`, Ausgabe nach `db/sqlc`). goose-Migrationen laufen produktiv
eingebettet (`db/migrate_pg.go`, `go:embed`); von Hand:
`goose -dir db/migrations postgres "$PLEXAMS_TEST_PG_URI" up|down-to 0|status`.
