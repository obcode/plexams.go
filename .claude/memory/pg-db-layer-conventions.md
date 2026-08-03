---
name: pg-db-layer-conventions
description: "Wie eine db-Methode auf PostgreSQL portiert wird: Dateilayout, Fehlersemantik, sqlc-Fallen, Sortierung, Singletons, jsonb — festgelegt in Phase 3a"
metadata:
  node_type: memory
  type: project
---

Die Konventionen, nach denen die 41 globalen Methoden in Phase 3a portiert wurden
([[postgres-migration]]). Für 3b–3e gilt dasselbe Muster — nicht neu erfinden.

## Dateilayout

| Was | Wohin |
| --- | --- |
| Query | `db/queries/<domäne>.sql` (eine Datei je `db/<domäne>.go`) |
| Methoden + Mapper | `db/<domäne>_pg.go`, neben der Mongo-Fassung |
| Tests | `db/<domäne>_pg_test.go`, `package db_test`, `pgtest.NewDB(t)` |

Mapper heißen `<typ>FromRow(row sqlc.X) *model.Y` und `<typ>sFromRows`. Schreibende
Methoden bauen die `sqlc.…Params`-Struct direkt, ohne `fromModel`-Funktion — die
Feldnamen stimmen fast immer überein.

`db/schema_semantics_test.go` liefert die Helfer `exec`, `count`, `seedExamFixtures`;
`db/export_test.go` das `PoolForTest()` für Roh-SQL im Test.

## Fehlersemantik — die Mongo-Fassung entscheidet, nicht der Geschmack

- `pgx.ErrNoRows` bleibt **innerhalb** von `db/`. Nach außen entweder `(nil, nil)`
  oder ein Fehler — **genau so, wie die Mongo-Methode es gemacht hat.**
- `Nta`, `StudyProgram`, `GetUserByEmail`, `GetPlaner`, `Get*Config`,
  `GetActiveSemester`, `GetSchedulerState` → `(nil, nil)`.
- `RoomByName` → **Fehler** (`cannot find room %s`). `plexams.UpdateRoom` und
  `BlockRoomForSlot` lesen genau diesen Fehler als „gibt es nicht".
- Listen immer `make([]*T, 0, len(rows))`, Maps immer `make(map[…], len(rows))`.
  `emit_empty_slices` deckt nur die von sqlc erzeugten Slices ab, nicht die
  Zielslices der Mapper und keine Maps.
- Ein `delete` mit Rückgabewert `bool` wird `:execrows` + `rows > 0`.
- Ein Replace, das laut Mongo **nicht** upsertet, bleibt ein `update … returning *`
  ohne `on conflict`.

## sqlc — drei Fallen, die alle lautlos sind

1. **`timestamptz` braucht ein Override**, sonst kommt `pgtype.Timestamptz`.
   `emit_pointers_for_null_types` erreicht es nicht. Der `db_type` heißt
   **`timestamptz`** — `pg_catalog.timestamptz` trifft nichts und schweigt.
   Festgenagelt in `db/sqlc_timestamptz_test.go`.
2. **`rename:` erwartet den von sqlc *berechneten* Namen**, nicht den Tabellennamen
   (`ntum: "NTARow"`, nicht `nta:`).
3. **Positionsparameter erzeugen `Column2`** o. ä., sobald ein Cast im Spiel ist.
   Dann `@name`-Parameter benutzen (`any(@mtknrs::text[])`).

## Sortierung

`ORDER BY` ist nie egal — Mongo sortierte binär.

- **Bezeichner** (Raumnamen, Kürzel, E-Mail-Adressen, Template-Namen):
  `order by x collate "C"`. `R1.006`/`R1.046`/`R10.1` kommen unter `de_DE` anders
  heraus.
- **Personennamen** (nta, permanent_non_invigilator): kein explizites `collate`,
  also Cluster-Default (heute `C.UTF-8`). So ist der Wechsel auf menschenfreundliche
  Sortierung eine Zeile und keine Suche.
- Wo Mongo **gar nicht** sortierte, ist eine Sortierung eine Verbesserung, keine
  Verhaltensänderung — aber im Query kommentieren.

## Wiederkehrende Muster

- **Singletons** (`planer`, `anny_config`, `generation_config`, `active_semester`,
  `scheduler_state`): `id int primary key check (id = 1)`, Schreiben als
  `insert … values (1, …) on conflict (id) do update set …`. Ein zweiter Datensatz
  ist damit unmöglich statt nur nie geschrieben.
- **Partielles Update** (Mongos `$set` einer Teilmenge): das `on conflict`-`do update`
  listet nur diese Spalten auf. Siehe `TouchSchedulerFire` — es darf das Ergebnis des
  letzten Laufs nicht löschen.
- **Abgeleitete Werte sind keine Spalten.** `room.needs_request` ist
  `generated always as (request_with <> 'NONE') stored`; `model.Planer.DefaultMail`
  und die vier `Effective*` haben gar keine Spalte. Genau ihre Speicherung hat in
  Mongo die zwei Schreibweisen von `needsRequest` auseinanderlaufen lassen.
- **jsonb + `format_version`**: beim Lesen prüfen und bei Unbekanntem hart
  fehlschlagen. Die `json`-Tags sind gleichzeitig der GraphQL-Vertrag, ein Rename in
  der `.graphqls` ändert also still das Speicherformat. Ohne Prüfung kämen die
  Werte auf Null zurück und der Generator liefe damit. Round-Trip-Test füllt alle
  Felder **per Reflection**, sonst rutscht ein neu hinzugefügtes Feld als Nullwert
  durch.
- **Fremdschlüssel dürfen Löschen verbieten.** `DeleteStudyProgram` scheitert jetzt,
  solange Prüfungen oder Primuss-Daten den Kürzel referenzieren — mit einer Meldung,
  die das Programm nennt, weil sie unverändert in der GUI landet
  (SQLSTATE `23503` abfangen, nicht den nackten Constraint-Fehler durchreichen).

## Was in 3b dazukam

- **`SELECT DISTINCT` und `collate "C"` vertragen sich nicht.** Weder als
  `order by x collate "C"` (Fehler 42P10) noch mit dem Collate in der Select-Liste
  (sqlc verliert dann den Spaltentyp und liefert `[]interface{}`). Lösung: das
  `distinct` in eine Unterabfrage, sortieren außen.
- **`ON UPDATE CASCADE`, wo der Schlüssel ein fachlicher Wert ist.** Der
  Ancode-Rename ist damit ein `UPDATE` statt dreier Schritte. Gilt nur für
  abgeleitete Quelldaten (`primuss_count`, `primuss_conflict`) — für
  handgepflegte Tabellen bleibt es bei der Regel aus [[postgres-migration]].
- **`DeleteOne` ist nicht `DELETE … WHERE`.** Wo Mongo genau ein Dokument
  gelöscht hat, muss PG genau eine Zeile löschen
  (`delete … where id = (select id … limit 1)`). Bei `RemoveStudentReg` ist das
  der Unterschied zwischen „Dublette entfernen" und „beide Anmeldungen weg".
- **Sortierung über das jsonb-Dokument** (`order by student ->> 'name'`) statt
  über eine duplizierte Spalte — dann kann nichts auseinanderlaufen.
- **Zwei gleichnamige Felder sind nicht dasselbe Feld.** Siehe
  `studentreg.program` vs. `student_program`. Beim Portieren jeder Methode
  prüfen, woher der Wert in Mongo *wirklich* kam: Collection-Name oder
  Dokumentfeld.
- Testnamen kollidieren mit den Mongo-Tests im selben Package
  (`db_test`) — die PG-Variante bekommt einen inhaltlich anderen Namen, kein
  `…PG`-Suffix.

## Nicht portiert — und warum

- `EnsureIndexes` — stirbt mit Mongo, goose besitzt die Struktur.
- `SetRoomRequestWith` — **toter Code**, kein Aufrufer im ganzen Repo, und sein
  `needsRequest`-Parameter hätte in PG nichts mehr zu schreiben.
- `globalDatabase()` — Mongo-Interna.

Vollständigkeitsprüfung, die das belegt hat: alle `func (db *DB)` mit
`globalDatabase()` im Rumpf gegen alle `func (db *PG)` diffen. Für 3b–3e dasselbe
mit `getCollectionSemester`.
