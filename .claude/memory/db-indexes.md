---
name: db-indexes
description: EnsureIndexes — why plan(ancode) is a partial index and studentregs is deliberately not unique
metadata:
  node_type: memory
  type: project
---

Until 2026-07-30 **no collection had any index except `_id`**. Uniqueness was carried entirely by write patterns (drop+insert, ReplaceOne+upsert) and by after-the-fact checks in [plexams/validate_db.go](plexams/validate_db.go). `EnsureIndexes` in [db/indexes.go](db/indexes.go) now creates them, called from `loadSemesterMeta` ([plexams/semester_switch.go](plexams/semester_switch.go)) so it runs on startup **and** on every semester switch.

Unique: `plan(ancode)`, `joint_links(program, primussancode)`, `primuss_ancodes(ancode, primussancode.program)`, and `nta(mtknr)` in the global `plexams` DB. Lookup-only: `rooms_planned(ancode)`, `rooms_planned(starttime)`, `mutation_log(time desc)`.

Two decisions that came out of scanning the real databases — **do not "fix" either one**:

- **`plan(ancode)` is a PARTIAL index** on `ancode: {$exists: true}`. Databases from before the slotless refactor (2022-WS, 2023-SS) store plan entries as `daynumber/slotnumber/examgroupcode` with **no `ancode` field at all**. Without the filter every one of those documents collides on `null` and the index can never be created there. With it, the index builds on all existing semester databases. See [slotless-timebased-redesign.md](slotless-timebased-redesign.md).
- **`studentregs_<PROG>(AnCode, MTKNR)` is deliberately NOT unique**, only a lookup index. The Primuss source data really does contain a student registered twice for the same exam; it has been carried along across several semesters. A unique index would make the Primuss import *fail* instead of protecting it. Such duplicates belong in the validation report, not in a constraint. See [primuss-xlsx-import.md](primuss-xlsx-import.md).

`EnsureIndexes` is **best-effort and never fatal**: an index contradicted by existing data is logged as a warning naming the collection and retried on the next start. This is required — the server must start even against a database whose data is inconsistent, which is exactly when the planner needs `validate db` to diagnose it. Creating an existing index is a no-op.

Related: [db-integrity-validation.md](db-integrity-validation.md) lists the invariants that were previously only checked by hand; `plan(ancode)`, `nta(mtknr)` and the joint-link uniqueness are now enforced by the database. Tests in [db/indexes_test.go](db/indexes_test.go) cover the partial-filter case and the best-effort contract; they need a test MongoDB ([mongotest-without-docker.md](mongotest-without-docker.md)).
