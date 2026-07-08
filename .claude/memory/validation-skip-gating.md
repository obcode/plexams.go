---
name: validation-skip-gating
description: "Validators skip (not fail) when their planned data doesn't exist yet; ValidationReport gained skipped/skipReason"
metadata:
  node_type: memory
  type: project
  originSessionId: d9fba672-cc98-4a3f-8753-eec2dd6aa584
---

Validators that check *planned* data are now data-gated so they don't produce noise before that stage of planning. Added 2026-07-05 ([plexams/validate_preconditions.go](plexams/validate_preconditions.go)).

- Signal: room/invigilation gates use **actual DB contents** (`hasPlannedRooms` / `hasInvigilations`). The Terminplan gate uses the **`condExamScheduleGenerated` milestone** (`planGenerated`), NOT "any plan entry exists": EXaHM/SEB pre-planning fixes a few exams into slots before the full schedule is generated, so a plan-entry count would let conflicts/constraints run prematurely.
- conflicts + constraints → skip until the schedule is generated (`skipNoPlan` = "noch kein Terminplan generiert"); all `rooms-*` → skip when no planned rooms (`skipNoRooms`); all invigilation-* → skip when no invigilations (`skipNoInvigilations`).
- Short-circuit via `validation.skip(reason)` (in [plexams/validation_report.go](plexams/validation_report.go)): returns Ok=true, no findings, and `Skipped=true` + `SkipReason`.
- Schema: `ValidationReport` gained `skipped: Boolean!` and `skipReason: String` (additive, backward compatible). GUI should render skipped neutrally ("übersprungen"), NOT as a green pass. See [[gui-and-cli-sync]].
- 2026-07-08 extended: `ValidateStudentRegs` now skips when **no Primuss data imported** (`hasStudentRegs` = `CountStudentRegsPlanned > 0`; `skipNoStudentRegs`), and `ValidatePreplanAssignment` (the SEB/EXaHM Vorplanung query, returns `PreplanValidation` not `ValidationReport`) skips when `len(PreplanExams)==0` (`skipNoPreExams`). `PreplanValidation` gained its own `skipped`/`skipReason` fields + `skippedPreplanValidation()` helper in [plexams/preplan_assign.go](plexams/preplan_assign.go); `GeneratePreplanAssignment` empty case reuses it. Both were green-when-empty before; now neutral "übersprungen". GUI-agent must render `skipped` for `PreplanValidation` too.
- NOT gated (intentionally, user decision 2026-07-08): the 5 `ValidateDB*` referential-integrity validators — "solange es leer ist nur die Datenbank-Integrität; das macht Sinn auch noch ohne Daten". `ValidatePrePlannedExahmRooms` left as-is (early pre-booking check, ambiguous gate).

Complements the graded-severity work: [[validation-conflict-severity]], [[db-integrity-validation]], [[preplan-validation-severity]].
