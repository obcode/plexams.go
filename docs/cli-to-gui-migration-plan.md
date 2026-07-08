# Migrationsplan: CLI → GUI (GraphQL + plexams.gui)

> **Status 2026-07-08: ABGESCHLOSSEN.** Das Cobra-`cmd/` wurde entfernt; `plexams.go`
> ist ein reiner GraphQL/REST-Server (Bootstrap in `bootstrap/`, gestartet aus
> `main.go`, nur noch die Flags `-v`/`--db-uri`/`--semester`). Die letzten CLI-only
> Funktionen wurden ersetzt: Datei-Exporte laufen als REST-Downloads
> (`/download/pdf/{kind}`, `/download/csv/{kind}`, `/download/ics/{program}`), einzelne
> Primuss-Feindaten als Mutations (`addStudentReg`/`removeStudentReg`).
> Diagnose/Debug-Kommandos (`info …`, `invigilation problem`, das separate `zpa/cli`)
> und `ics import-mucdai` sind ersatzlos entfallen. Damit ist die frühere Divergenz zu
> `plan-slotless-timebased.md` aufgelöst: **das CLI bleibt NICHT** — es ist weg.
>
> Persistierter Plan aus der Konzeptanalyse vom 2026-06-17.
> Ziel: alles, was per `cmd/` CLI lief, über die GraphQL-API/REST ins Svelte-Frontend
> (plexams.gui) holen; Per-Semester-Konfiguration aus YAML in die DB verlagern.

## Eckentscheidungen

- **Bleibt lokales Single-User-Werkzeug** — nicht gehostet, nicht mehrbenutzerfähig.
  → **Keine Authentifizierung nötig**, localhost-only CORS bleibt.
- **Secrets bleiben in einer schlanken Config-Datei** (Bootstrap): `db.uri`/`db.database`,
  SMTP-, ZPA-, anny-Credentials, `server.port`. Alles andere wandert in die DB.
- **Kein Technologiewechsel.** GraphQL passt: Resolver sind ~90 % dünne Passthroughs
  zu `*plexams.Plexams`-Methoden. CLI und GraphQL sind beide nur Fassaden über derselben
  Fachlogik — wir bauen die GraphQL-Fassade aus, statt etwas wegzuwerfen.

## Sicherheits-Leitplanken (WICHTIG, laufende Planungsphase)

Stand 2026-06-17 ist eine reale Planung im Gange (Raum- und Aufsichtenplanung noch
nicht fertig; es kommen weiterhin Einträge in `semesterConfig`/`roomconstraints`).
Daraus folgt:

1. **Reihenfolge umgedreht:** additive Phasen (2 Write-API, 3 Exporte) ZUERST — sie
   ergänzen nur neue Mutations/Endpunkte und ändern weder YAML-Workflow noch
   DB-Schreibverhalten. Config-in-DB (Phase 1) ERST nach Abschluss der laufenden
   Raum-/Aufsichtenplanung, oder mit YAML-Fallback (YAML bleibt Quelle, DB überschreibt
   nur falls vorhanden).
2. **Neue Collection statt Bestehendes anfassen:** `semester_config` wird heute bei
   jedem Start gedroppt + neu geschrieben (nur Snapshot der *berechneten* Config). Die
   neue *Quell*-Config bekommt eine NEUE Collection `raw_config`. Nichts Bestehendes
   wird umgewidmet.
3. **Additivität:** Jede Phase-2-Mutation ruft exakt dieselbe `Plexams`-Methode wie die
   CLI heute → keine neuen DB-Schreibmuster.
4. **Backups/Tests:** Vor jeder DB-berührenden Arbeit `mongodump` der Semester-DB.
   Entwicklung gegen eine Kopie (`mongorestore` als `2026-SS-test`, via `--db-uri`/
   `db.database` ansteuern). Echt-DB wird beim Entwickeln nie berührt.
5. **CLI bleibt** als dünne Zweitfassade erhalten (Skripting, Migration, Notfall).

## Ausgangslage (gemessen 2026-06-17)

| Bereich | Stand |
|---|---|
| CLI-Commands | 92 (≈46 lesend / ≈46 mutierend) |
| GraphQL | 61 Queries, 22 Mutations (2 davon `panic`/Stub), 0 Subscriptions |
| Write-Coverage GraphQL vs CLI | ~45–50 % |
| Auth | keine (nur CORS auf localhost) — bleibt so |
| File-Upload/-Download | nicht unterstützt |
| Config | ~85 Viper-Keys, ~65–70 % davon pro Semester dynamisch |

## Die vier Querschnittsthemen, die die CLI „gratis" löst

1. **Interaktive Bestätigungen** (`confirm()`, 16 Cmds) — trivial: macht künftig die GUI.
2. **Reine Mutationen** (plan/primuss/prepare) — einfach, dünne Wrapper um bestehende Methoden.
3. **Datei-Output** (37 Cmds PDF/CSV/ICS/JSON) — GraphQL ungeeignet → REST-Download-Endpunkte
   am vorhandenen chi-Router.
4. **Lange Läufe** (ZPA-Import, `invigilation generate`/Simulated Annealing, Batch-prepare,
   Massen-email) — Job-Pattern (Mutation startet Job → Status via Polling/Subscription).
   (Auth als 5. Thema entfällt durch Single-User-Entscheidung.)

---

## Empfohlene Reihenfolge (sicherheitsangepasst)

1. **Phase 2 — Write-API vervollständigen** (jetzt, additiv, niedriges Risiko)
2. **Phase 3 — Datei-Exporte** (jetzt, additiv, entkoppelt)
3. **Phase 1 — Config in die DB** (nach laufender Planung, oder mit YAML-Fallback)
4. **Phase 4 — Job-Pattern für lange Läufe**
5. **Phase 5 — E-Mail/ZPA-Upload (mit dryRun, als Jobs)**

---

## Phase 2 — Write-API vervollständigen

Muster (wie `graph/constraints.resolvers.go`): jede Mutation ist ein Einzeiler-Wrapper.

```go
func (r *mutationResolver) AddExamToSlot(ctx context.Context, day, time, ancode int) (bool, error) {
    return r.plexams.AddExamToSlot(ctx, day, time, ancode) // Methode existiert (CLI nutzt sie)
}
```

- **2.1 Stubs:** `addExamToSlot`, `rmExamFromSlot` (heute `panic`).
- **2.2 Plan:** `moveExamToSlot`, `addExamToSlottime` (other-fk), `changeRoom`, `lockExam`/
  `unlockExam`, `lockPlan`, `preAddExamToSlot`, `preAddRoomToExam`.
- **2.3 Primuss:** `addAncode`, `fixAncode`, `addStudentReg`, `rmStudentReg`.
- **2.4 Prepare:** `connectExam`, `addMucDaiExam`; kurze als Mutation, lange
  (`prepareGeneratedExams`, …) später als Job (Phase 4).
- **2.5 Invigilation:** `addInvigilation`/`preAddInvigilation` (confirm macht die GUI).

Workflow: Schema in `graph/*.graphqls` ergänzen → `go generate ./...` → Einzeiler füllen.
Aufwand: niedrig pro Mutation, größter sichtbarer Fortschritt.

## Phase 2b — Validierung ins GUI (read-only, additiv, hoher Nutzen)

Live-Validierung beim Planen ist einer der wertvollsten GUI-Gewinne. ABER: die 17
`Validate*`-Methoden geben heute nur `error` zurück und DRUCKEN Befunde auf die Konsole
(yacspin-Spinner, aurora-Farben, intern `validationMessages []string`). Für das GUI müssen
sie strukturierte Ergebnisse ZURÜCKGEBEN statt zu drucken.

Muster:
```go
type ValidationResult struct {
    Validator string                // "constraints", "conflicts", "rooms-per-slot", ...
    Findings  []*ValidationFinding
}
type ValidationFinding struct {
    Level   string                  // error | warning | info
    Ancode  *int
    Message string
}
```
- Jede `Validate*` baut `[]*ValidationFinding` auf und gibt sie zurück.
- CLI druckt aus dem Slice (eine Druckstelle, Farben/Spinner bleiben) → CLI unverändert nutzbar.
- GraphQL: read-only Queries — `validateConstraints`, `validateConflicts(...)`, `validateRooms`,
  `validateInvigilation`, `validateAll`.
- `--sleep`/Watch-Mode → GUI-Validierungs-Panel, das auf Knopfdruck oder automatisch nach
  jeder Planungs-Mutation neu lädt (sofortiges Konflikt-Feedback beim Verschieben).
- Read-only → sicher während laufender Planung. Aufwand: mittel (17× print→return-Refactor,
  mechanisch); Nebeneffekt: saubere Trennung Logik/Ausgabe in der CLI.

## Phase 3 — Datei-Output (PDF/CSV/ICS/JSON)

REST-Endpunkte am chi-Router in `graph/server.go`:

```go
r.Get("/export/pdf/{kind}",    plexams.HTTPHandlePDF)
r.Get("/export/csv/{kind}",    plexams.HTTPHandleCSV)
r.Get("/export/ics/{program}", plexams.HTTPHandleICS)
r.Get("/export/json/{kind}",   plexams.HTTPHandleJSON)
```

Generier-Funktionen auf `io.Writer` refaktorieren → dieselbe Funktion bedient CLI (Datei)
und HTTP (`http.ResponseWriter` + `Content-Disposition`). Im GUI Download-Links.

## Phase 1 — Config in die DB

### 1.1 Bootstrap/DB-Split
Schlanke `.plexams.yaml` behält nur: `semester`, `db.*`, `smtp.*`, `zpa.{baseurl,username,
password}`, `anny.{token,personalization_name}`, `server.port`. Rest → DB.

### 1.2 Neue Collection `raw_config` + `RawSemesterConfig`
Rohe Eingabe-Struktur (nicht die berechnete!), ersetzt verstreute `viper.Get(...)`:
`From/FromFK07/Until`, `DayNumberStart`, `Slots []string`, `GoDay0`, `ForbiddenDays`,
`GoSlots [][]int`, `Emails`, `AdditionalExamer`, `roomconstraints`, `duration`,
`donotpublish`, `publish.additionalExams`, `knownConflicts`, `specialInterests`,
`invigilation.optimizer.*`, Programm-Listen, `coverPages`, `rooms.timelag`, `planer`.

### 1.3 `Plexams`-Refactor + `Reload()`  (heikelster Einzelschritt)
- `setSemesterConfig`/`setGoSlots` lesen NICHT mehr aus viper, sondern aus `*RawSemesterConfig`.
  Rechenlogik (GoDay0-Offset, Planning-Window ab `fromFK07`, `allDays`/`allSlots`) **identisch**.
- Neue Methode `ReloadSemesterConfig(ctx)`: aus `raw_config` laden → neu berechnen → `setRoomInfo`.
- `NewPlexams` ruft `ReloadSemesterConfig`; fehlt DB-Config (frisches Semester) → leere Defaults.
- File-Watcher in `cmd/root.go` entfällt; jede Config-Mutation ruft am Ende `ReloadSemesterConfig`.
- **Nebenläufigkeit:** config-tragende Felder in `atomic.Pointer[configState]` kapseln
  (lockfreier atomarer Tausch beim Reload). Alternative: `sync.RWMutex`.
- **Test:** gleiche Inputs → identische `allSlots` wie die alte viper-Variante (Golden-Test).
- **YAML-Fallback während Übergang:** falls `raw_config` leer, aus YAML lesen → so bleibt
  der laufende Workflow heil, bis bewusst umgeschaltet wird.

### 1.4 GraphQL Config-API
`graph/config.graphqls`: `Query.rawSemesterConfig`; pro Sektion eine Mutation
(`setSemesterDates`, `setSlots`, `setForbiddenDays`, `setGoSlots`, `setEmails`,
`setInvigilationOptimizer`, `setRoomConstraints`, …). Jede: persistieren →
`ReloadSemesterConfig` → aktualisierte Config zurückgeben.

### 1.5 GUI
Seite „Semester-Konfiguration" mit Formularblöcken pro Sektion, Client-Validierung.

### 1.6 Migration
Einmal-Command `plexams.go migrate config-to-db`: liest aktuelle YAML via viper,
schreibt als `RawSemesterConfig` in `raw_config`. Danach DB = Quelle.

## Phase 4 — Job-Pattern für lange Läufe

`Job{ID, Kind, Status(pending|running|done|failed), Progress, Message, StartedAt,
FinishedAt, Error}` in neuer `jobs`-Collection. Mutation startet Goroutine, kehrt sofort
mit `pending` zurück. Fortschritt: **Polling** (`Query.job(id)`, alle 1–2 s) zuerst —
ausreichend für Single-User; GraphQL-Subscription (Websocket) optional später.
Anwenden auf: ZPA-Import, `invigilation generate`, Batch-prepare, Massen-email, upload-plan.

## Phase 5 — E-Mail / ZPA-Upload

Mutations mit `dryRun: Boolean!` (= heutiges `--run`/`--dryrun`), Bestätigung in GUI,
laufen als Jobs (Phase 4).

---

## Offene Detail-Entscheidungen

- Phase 1.3: `atomic.Pointer[configState]` (empfohlen) vs. `sync.RWMutex`.
- Phase 4: Polling (empfohlen, schlank) vs. Subscriptions.
- Phase 3: REST-Download (empfohlen) vs. GraphQL-Mutation mit URL-Rückgabe.

## Erste konkrete Arbeitspakete (wenn es losgeht)

Empfohlener Start (additiv, sicher während Planung): **Phase 2**, beginnend mit den
zwei `panic`-Stubs `addExamToSlot`/`rmExamFromSlot`, dann die Plan-Mutations.
Vorher: `mongodump` + Test-DB-Kopie anlegen.
