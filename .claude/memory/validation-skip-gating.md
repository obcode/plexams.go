---
name: validation-skip-gating
description: "Validators skip (not fail) when their planned data doesn't exist yet; ValidationReport gained skipped/skipReason"
metadata:
  node_type: memory
  type: project
  originSessionId: d9fba672-cc98-4a3f-8753-eec2dd6aa584
---

Validators that check *planned* data are now data-gated so they don't produce noise before that stage of planning. Added 2026-07-05 ([plexams/validate_preconditions.go](plexams/validate_preconditions.go)).

- Signal is the **actual DB contents** (self-correcting), not the planning-state milestone: helpers `hasPlanEntries` / `hasPlannedRooms` / `hasInvigilations`.
- conflicts + constraints → skip when no plan entries (`skipNoPlan`); all `rooms-*` → skip when no planned rooms (`skipNoRooms`); all invigilation-* → skip when no invigilations (`skipNoInvigilations`).
- Short-circuit via `validation.skip(reason)` (in [plexams/validation_report.go](plexams/validation_report.go)): returns Ok=true, no findings, and `Skipped=true` + `SkipReason`.
- Schema: `ValidationReport` gained `skipped: Boolean!` and `skipReason: String` (additive, backward compatible). GUI should render skipped neutrally ("übersprungen"), NOT as a green pass. See [[gui-and-cli-sync]].
- NOT gated (intentionally): the 5 `ValidateDB*` and `ValidateStudentRegs` (work on DB/Primuss data independent of planning; empty = clean). `ValidatePrePlannedExahmRooms` left as-is (early pre-booking check, ambiguous gate).

Complements the graded-severity work: [[validation-conflict-severity]], [[db-integrity-validation]], [[preplan-validation-severity]].
