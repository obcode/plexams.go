---
name: db-indexes
description: EnsureIndexes — why plan(ancode) is a partial index and studentregs is deliberately not unique
metadata:
  node_type: memory
  type: project
---

Until 2026-07-30 **no collection had any index except `_id`**. Uniqueness was carried entirely by write patterns (drop+insert, ReplaceOne+upsert) and by after-the-fact checks in [plexams/validate_db.go](plexams/validate_db.go). `EnsureIndexes` in [db/indexes.go](db/indexes.go) now creates them, called from `loadSemesterMeta` ([plexams/semester_switch.go](plexams/semester_switch.go)) so it runs on startup **and** on every semester switch.

Unique: `plan(ancode)`, `joint_links(program, primussancode)`, `primuss_ancodes(ancode, primussancode.program)`, and `nta(mtknr)` in the global `plexams` DB. Lookup-only: `rooms_planned(ancode)`, `rooms_planned(starttime)`, `mutation_log(time desc)`.

One decision that came out of scanning the real databases — **do not "fix" it**:

**`studentregs_<PROG>(AnCode, MTKNR)` is deliberately NOT unique**, only a lookup index. The Primuss source data really does contain a student registered twice for the same exam. This is *not* a legacy artefact: it is present in the current semester and in the freshly imported workspace, so it would come straight back with newly imported data. A unique index would make the Primuss import *fail* instead of protecting it. Such duplicates belong in the validation report, not in a constraint. See [primuss-xlsx-import.md](primuss-xlsx-import.md).

Scope decision (2026-07-30, from the maintainer): the optimizations only have to work with **newly imported data** — pre-slotless archives (2022-WS, 2023-SS, whose plan entries carry `daynumber/slotnumber/examgroupcode` and no `ancode`) may be ignored. `plan(ancode)` is therefore a plain unique index; on those archives its creation fails and is logged, which is fine because they are never written to. Do not reintroduce a partial filter to accommodate them. See [slotless-timebased-redesign.md](slotless-timebased-redesign.md).

`EnsureIndexes` is **best-effort and never fatal**: an index contradicted by existing data is logged as a warning naming the collection and retried on the next start. This is required — the server must start even against a database whose data is inconsistent, which is exactly when the planner needs `validate db` to diagnose it. Creating an existing index is a no-op.

Related: [db-integrity-validation.md](db-integrity-validation.md) lists the invariants that were previously only checked by hand; `plan(ancode)`, `nta(mtknr)` and the joint-link uniqueness are now enforced by the database. Tests in [db/indexes_test.go](db/indexes_test.go) cover the partial-filter case and the best-effort contract; they need a test MongoDB ([mongotest-without-docker.md](mongotest-without-docker.md)).
