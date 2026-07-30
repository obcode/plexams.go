---
name: db-migrations
description: Schema migration framework for semester databases; how to add a migration and what version 2 did
metadata:
  node_type: memory
  type: project
---

Semester databases now have real migrations ([db/migrations.go](db/migrations.go), added 2026-07-30). Before that there was no way to evolve a database's layout: `SemesterMeta.SchemaVersion` existed but `currentSchemaVersion` sat at 1 forever, so collection renames in the code silently orphaned their data under the old name.

**Adding one:** append an entry to `migrations()` with the next `version`, bump `CurrentSchemaVersion` in the same file, write the step so it is **idempotent** (a crash between running a step and stamping its version re-runs it). Never edit or reorder a released entry. `MinSupportedSchemaVersion` stays at the oldest layout the code can still *read* — bump it only to drop support deliberately.

`Migrate` is called from `loadSemesterMeta` ([plexams/semester_switch.go](plexams/semester_switch.go)), so it runs on startup **and** on every semester switch, **before** `EnsureIndexes` so indexes land on the migrated collections. Failure is logged, not fatal — the planner has to reach the GUI to diagnose it. The version constants live in `db` next to the migrations; `plexams` aliases them.

Deliberately skipped, do not "fix":

- **Databases without `semester_meta`** — they predate versioning *and* the slotless refactor (2021-WS … 2023-WS) and are archives, not planning targets.
- **Read-only databases** — the protection wins. They stay usable because `MinSupportedSchemaVersion` still covers them; unprotect to migrate.
- **Databases stamped newer than the code** — guards against a downgrade rewriting data.

**Version 2** renames the collections the code stopped reading: `mucdai_links` and `mucdai_<PROG>` → `joint_*` (see [mucdai-to-joint-generalization.md](mucdai-to-joint-generalization.md)), `generated_exams`/`generated_exams_state` → `assembled_exams*`. All pairs are structurally identical, so no document is transformed. This was not cosmetic: one workspace had **42 joint links unreachable** under the old name while the code read an empty `joint_links`, plus the per-program collections and the assembled-exams cache in several semesters.

`renameCollection` never overwrites: an empty target is dropped and then replaced (exactly the state a partial rename leaves behind — the code creates the new collection on first write while the data stays under the old name), a target holding data leaves both in place and logs for manual resolution. The per-program regex requires 2–4 uppercase letters, so `mucdai_links` cannot be caught by it — there is a test for that.

Related: [db-indexes.md](db-indexes.md) (runs right after), [semester-dump-restore.md](semester-dump-restore.md), [fresh-db-bootstrap.md](fresh-db-bootstrap.md).
