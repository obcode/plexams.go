---
name: csv-datasets-export-import
description: Human-readable CSV export/import of entered data with ABSOLUTE times (period-shift robust); separate from the JSON semester dump
metadata:
  node_type: memory
  type: project
  originSessionId: 1b1817c2-9db2-4b1a-8f9c-075a2da64368
---

Feature (branch `feat/csv-export`, off main, pushed 2026-07-06): human-readable **CSV**
round-trip of the data Oliver enters by hand, in addition to (not replacing) the JSON
semester dump [[semester-dump-restore]]. Files: `plexams/csv_export.go` (+test),
`db.UpsertPreplanExam` in `db/preplan_exams.go`, routes in `graph/server.go`, CLI in
`cmd/export.go`/`cmd/import.go`.

**Why CSV existed at all:** the JSON dump stores period-relative slot numbers; when the
exam period shifts, those slots point at the wrong dates. CSV stores **absolute
date/time** and re-import recomputes the slot in the CURRENT period via
`SetExternalExamTime → AddExamToSlottime → getSlotForTime(time, duration)`
(plexams.go:256). That's the whole point.

**Datasets** (allow-list, order in `csvDatasetOrder`): constraints (incl.
notPlannedByMe), external-exams (master data only), **exam-times** (all plan entries
with ExternalTime = external + notPlannedByMe MUC.DAI, absolute date+time),
preplan, room-requests, exams-to-plan, duration-overrides, conflict-ratings,
can-share-slot.

**Endpoints:** `GET /download/dataset-csv?name=`, `GET /download/my-inputs-csv.zip`
(all as ZIP of CSVs), `POST /upload/dataset-csv` (multipart name+file; WritesAllowed +
read-only guarded). CLI: `export dataset-csv --name`, `export my-inputs-csv`,
`import dataset-csv --name`.

**Import safety (after the earlier silent-wipe incident):** row-keyed datasets **upsert
per row** (missing rows untouched, never drop the collection); the only full-replace one
(room-requests) refuses an empty file; every import validates the CSV **header matches
the dataset** (guards against wrong-file upload). CSV uses UTF-8 BOM for Excel, "," delim,
";" for in-cell lists, dd.mm.yyyy / dd.mm.yyyy HH:MM dates. Preplan keeps its id via
UpsertPreplanExam (notSameSlot/canShareSlot reference ids). Caveats documented in code:
room-request day/slot stay period-relative; preplan embedded Constraints not exported.
No live Mongo in dev env → only pure CSV framing/format helpers unit-tested.
