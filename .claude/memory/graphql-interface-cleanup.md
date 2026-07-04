---
name: graphql-interface-cleanup
description: GraphQL schema audited vs GUI usage and trimmed; over-trimmed 3 still-used queries (examsToPlan page), restored 2026-07-04.
metadata:
  node_type: memory
  type: project
  originSessionId: da8a958a-7cc3-4731-88bb-90c5303bc30c
---

The GraphQL API was audited against actual plexams.gui usage (2026-07) and the unused surface was removed on `main` via fast-forward (linear history, commits `9f7a2ec`, `c8a97f4`, `d56ef6c`).

Removed: all 10 GUI-unused **Mutations** (addExamToSlot stub; excludeDays/possibleDays/sameSlot/placesWithSockets/rmConstraints superseded by `addConstraints`; zpaExamsToPlan; the 3 one-off config→DB migrations incl. CLI `invigilation migrate-constraints`) and all 32 GUI-unused **Queries**, together with the now-dead `*Plexams`/`db` methods behind them (kept anything with a live internal/CLI/test caller — e.g. `GetZpaExamsToPlan`, `RmConstraints`, `db.PreplanExam`).

The 3 backend-complete validators that were the only "wire-up" item — `validateStudentRegs`, `validateDB`, `validatePrePlannedExahmRooms` (parameterless `LogLine!` subscriptions ending with a `ValidationReport`) — have since been wired into the GUI. Audit fully closed. See [[gui-and-cli-sync]].

**Correction (2026-07-04):** the audit was NOT accurate — it over-trimmed, and did so more than once. The "GUI usage" check missed page-level SvelteKit `+page.server.js` load queries, so several still-used queries were deleted and their GUI pages threw GraphQL 400 at load. **7 queries restored end-to-end** (schema + resolver + `*Plexams` method + `go generate`; build/vet/`go test ./plexams/` green):
- examsToPlan page (`exam/examsToPlan/+page.server.js`): `zpaExamsNotToPlan`, `zpaExamsPlaningStatusUnknown` (zpa.graphqls / `plexams/zpa_get.go`), `examDurationOverrides` (exam_duration.graphqls / `plexams/exam_duration.go`; exported `ExamDurationOverrides`, distinct from internal `examDurationOverridesMap`).
- conflicts page: `examScheduleConflicts`, `studentConflictDecisions`, `examsCanShareSlot`, `canShareSlotSuggestions` (all exam_conflict.graphqls / `plexams/exam_conflict.go`; helpers `examInfoMap`/`examPair`/`sameSlotGroups`/`firstProgram`/`conflictcalc.NormPair` had survived).

**RESOLUTION (2026-07-04):** a GUI-side `grep -rlw` of the full removed list flagged essentially ALL 31 removed queries as still-referenced — the "GUI-unused" premise was broadly wrong. So the whole query-removal was undone: reverse-applied the hand-written hunks of `c8a97f4` (schema+resolvers) and `d56ef6c` (backend methods) — `git show <c> -- <paths, excluding graph/generated + models_gen> | git apply -R` — then `go generate`. All 31 queries + their `*Plexams`/`db` backend methods are back; build/vet/`go test ./plexams/` green. (The 7 already hand-restored for the two reported errors were kept and excluded from the reverse-apply.) The `graph/generated/generated.go` + `models_gen.go` were NOT patched — regenerated from the restored schema instead.

**Mutations (`9f7a2ec`): CORRECTLY removed — confirmed, left as-is.** It dropped 7 mutations: the setters `excludeDays`/`possibleDays`/`sameSlot`/`placesWithSockets`/`rmConstraints` (superseded by `addConstraints`), the `addExamToSlot` stub, and `zpaExamsToPlan`; plus 3 one-off migrations (incl. CLI `invigilation migrate-constraints`). The GUI `-w` grep flagged some as used, but the discriminating grep `grep -rnE "\b(name)\s*\(" src/` (a mutation CALL has a `(` after the name; a field selection does not) returned EMPTY for all 7 → none are actually called. The earlier hits were field-name collisions (`excludeDays`/`possibleDays`/`sameSlot` are still-valid Constraints fields). So the query-removal was buggy but the mutation-removal was sound. Net: only the 31 queries needed restoring.

Lesson: never delete a GraphQL query on a "GUI usage" grep that only covered `.svelte` — SvelteKit load funcs live in `+page.server.js`. And a plain word-grep over-reports because query field names collide with object-type field names.
