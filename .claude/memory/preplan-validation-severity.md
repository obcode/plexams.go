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

**`validatePrePlannedExahmRooms` was REMOVED 2026-07-05** (commit on main). It validated real ZPA/assembled exams against manual `rooms_pre_planned` assignments — the wrong pipeline; it never touched `preplan_exams` and did not match how the SEB/EXaHM pre-planning actually works. Gone: backend fn/file `validate_exahm_rooms.go`, subscription `validatePrePlannedExahmRooms`, resolver, CLI `validate preplanned-exahm-rooms`.

Intended validation structure (user's reorg):
- **SEB/EXaHM-Vorplanung** = `validatePreplanAssignment` (Query, graded `PreplanValidation`). Validates `preplan_exams` — the phase -1 pre-exams, **entirely without ZPA exams**. Belongs BEFORE the room planning as its own validation point (not under room planning).
- **Primuss** = `ValidateStudentRegs` (`student-regs`) — students with regs in >1 program (all info, [[validation-conflict-severity]] style). Its own validation point.

GUI TODO: drop the `validatePrePlannedExahmRooms` subscription; add `validatePreplanAssignment` as the early SEB/EXaHM-Vorplanung point; keep `validateStudentRegs` as its own Primuss point. See [[gui-and-cli-sync]].
