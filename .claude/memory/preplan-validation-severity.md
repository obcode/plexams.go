---
name: preplan-validation-severity
description: EXaHM/SEB Anny pre-plan validation now graded; small SEB in R-building are warnings not failures
metadata:
  node_type: memory
  type: project
  originSessionId: d9fba672-cc98-4a3f-8753-eec2dd6aa584
---

`validatePreplan`/`ValidatePreplanAssignment` (Anny pre-planning of EXaHM/SEB, [plexams/preplan_assign.go](plexams/preplan_assign.go)) used to return a flat `Messages []string` with `ok = len(messages)==0`, so ANY note failed it. The solver deliberately leaves small SEB exams (ExpectedStudents ≤ rBauSebThreshold) without an Anny slot — they go into the R-building — which is by design, but it was forcing `ok=false`.

Fixed 2026-07-05: findings are now graded. `PreplanValidation` gained `findings: [{level, message}]` (reuses the `ValidationLevel` INFO/WARNING/ERROR enum); `messages` + `ok` kept for backward compat (additive). Mapping:
- small-SEB-in-R-building → **warning** (user: "das sind allerhöchstens Warnings")
- genuine capacity shortfalls (not enough booked Anny seats, slot overflow, missing bookings, no Anny rooms) → **error**
- `ok` = no error-level findings (warnings/infos don't fail).

GUI should render `findings` by level and treat `ok` as "no errors" (see [[gui-and-cli-sync.md]]); the query is GUI-only (no CLI). Related: [[two-phase-exahm-seb]], [[preplanning-seb-exahm]], [[validation-conflict-severity]].
