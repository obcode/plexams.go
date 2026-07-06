---
name: semester-dump-restore
description: "Backup/restore feature — download whole semester as ZIP + per-page dataset download/upload, re-upload into fresh workspace DB for testing"
metadata:
  node_type: memory
  type: project
  originSessionId: 1b1817c2-9db2-4b1a-8f9c-075a2da64368
---

Feature (branch `feat/semester-dump-restore`, pushed 2026-07-06): dump a semester and
re-upload it into a fresh workspace DB for testing. Files: `db/dump.go`,
`plexams/dump.go` (+`dump_test.go`), routes in `graph/server.go`, CLI in
`cmd/export.go` + new `cmd/import.go`.

**Endpoints** (guards: WritesAllowed + read-only on all uploads):
- `GET /download/semester-dump.zip` — full per-semester clone: every collection →
  one canonical MongoDB-Extended-JSON file (wrapped in `{documents:[...]}` because
  ext JSON can't be a top-level array) + `manifest.json`.
- `POST /upload/semester-dump.zip` — restore into current DB; **refused unless empty**
  (ErrDatabaseNotEmpty → HTTP 409). Bookkeeping collections don't count as non-empty:
  semester_config_input/semester_config/semester_meta/mutation_log/sync_log.
  `semester_meta` is never overwritten (keeps workspace identity/read-only).
- `GET /download/dataset?name=` and `POST /upload/dataset` (multipart name+file) —
  per-page datasets from an allow-list: constraints (incl. notPlannedByMe),
  external-exams, preplan, mucdai-links, room-requests.

**external-exams dataset is special**: times live in the shared `plan` collection, so
it carries `non_zpaexams` (full replace) + the `plan` entries **filtered by external
ancode**; restore deletes plan entries for union(current+incoming external ancodes)
then re-inserts — never drops the whole plan/schedule.

**Two file formats (interoperable on upload):** dataset download = `{manifest,collections:{<coll>:[...]}}`;
semester-ZIP per-collection file = bare `{documents:[...]}`. RestoreDataset accepts **both** for
single-collection datasets (constraints/preplan/mucdai-links/room-requests), so a `constraints.json`
extracted from the semester ZIP uploads fine on the Constraints page. **Guard (data-loss fix
2026-07-06):** resolve all docs first and error "nothing changed" if the file has no recognized data
— earlier a format mismatch did `ReplaceRawCollection(coll, nil)` = drop + insert 0 = silent wipe.
Download filenames use `DatabaseName()` (physical DB), not the logical semester.

CLI mirrors: `export semester-dump -o f.zip`, `export dataset --name x -o f.json`,
`import semester-dump f.zip`, `import dataset --name x f.json`.

Type fidelity uses **canonical** ext JSON (`MarshalExtJSON(v,true,false)`) so ObjectID/
date/int32/int64 round-trip exactly; verified by unit tests (no live Mongo in dev env).
Workflow: `CreateWorkspace` → `SwitchSemester` → upload dump. See [[gui-and-cli-sync]],
[[cli-to-gui-migration]], [[primuss-xlsx-import]] (upload-endpoint precedent).
