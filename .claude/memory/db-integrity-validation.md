---
name: db-integrity-validation
description: ValidateDB split into 5 referential-integrity validators (plan/constraints/rooms/ntas/references)
metadata:
  node_type: memory
  type: project
  originSessionId: d9fba672-cc98-4a3f-8753-eec2dd6aa584
---

The old single `ValidateDB` (only checked "one plan entry per ancode") was replaced on 2026-07-05 by 5 focused referential/structural-integrity validators in [plexams/validate_db.go](plexams/validate_db.go). They complement the planning-*quality* validators (conflicts, rooms-per-slot, invigilation, zpa) — this layer checks the DB is internally consistent (no orphans/dangling refs/impossible states/stale rows after a move).

- `ValidateDBPlanEntries` — dup ancode; ancode is a real exam (to-plan∪external); slot exists in config; a non-slotted entry (0/0) must carry an ExternalTime. NOTE: an entry may legitimately have BOTH a slot and an ExternalTime — external exams whose time falls inside the exam period get a real slot too (AddExamToSlottime in plan.go). Do NOT flag slot+ExternalTime as a conflict.
- `ValidateDBConstraints` — orphan constraint; **FixedDay honored** (this closes the FIXME in ValidateConstraints; FixedTime/SameSlot/Exclude/PossibleDays stay in ValidateConstraints, not duplicated).
- `ValidateDBRooms` — the big TODO: planned_rooms ↔ plan-entry slot sync (stale-after-move), room in global list & active, seated students registered, no student seated twice per ancode, NtaMtknr → existing non-deactivated NTA, reserve rooms have no students.
- `ValidateDBNtas` — every NTA has Mtknr (closes the validate.go:18 TODO); plausible DeltaDurationPercent; no dup Mtknr.
- `ValidateDBReferences` — duration overrides / canShareSlot pairs / mucdai_links point to existing ancodes; unresolved links = info.

Wiring: each is a subscription field `validateDBPlanEntries|Constraints|Rooms|Ntas|References` (validation.graphqls) via `runValidation`; CLI `validate db` runs all 5 (see `dbValidations` slice in cmd/validate.go), `validate all` too. Shared helpers on Plexams: `knownAncodes`, `validSlots`, `dayDates`, `regsPerAncode`. See [validation-conflict-severity.md](validation-conflict-severity.md) and [graphql-interface-cleanup.md](graphql-interface-cleanup.md). GUI needs the 5 new subscriptions wired ([gui-and-cli-sync.md](gui-and-cli-sync.md)).
