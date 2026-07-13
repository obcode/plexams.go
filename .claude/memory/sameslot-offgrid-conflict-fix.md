---
name: sameslot-offgrid-conflict-fix
description: "slotless residue bug — solver+allowedSlots still grid-indexed, dropped off-grid foreign exams; fixed time-based + hard cross-campus travel buffer"
metadata:
  node_type: memory
  type: project
  originSessionId: cb67cae5-5daf-4ee8-8076-aa8d3ba43aa3
---

Bug found 2026-07-12 via the new spread statistics ([[spread-statistics]]): a student had two
overlapping exams (WiMa II 424 @10:30 mine, Kostenrechnung 338 @11:00 external/FK10, DB
Test26SS-v2 :27013) that neither the Terminplan solver nor validation flagged.

Root cause = incomplete [[slotless-timebased-redesign]]: conflict *detection* (validateStudentReg,
ExamScheduleConflicts, spreadcalc) was moved to absolute-time `conflictcalc.TimeProximity`, but two
places still modelled placement as membership in the fixed slot-start grid (`:30` slots). A foreign/
external exam placed at an off-grid time (11:00) was silently DROPPED:
- solver `buildExamPlanProblem`: `slotIndexAt(11:00)` miss → `continue` → obstacle vanished → no hardSep.
- `AllowedSlots`/`slotsWithConflicts`: `SlotForAncode(11:00)` = nil → excluded no slot → 10:30 stayed allowed.
- validation caught it only with onlyPlannedByMe=false; onlyPlannedByMe=true dropped it (338 foreign).
The new spread stats caught it because it reads plan-entry absolute times directly.

Fix (on working tree, not yet committed):
- `slotsWithConflicts` → time-based `overlapSlots` + `allowedSlotsFor`/`placedExamInfos` in plan.go:
  exclude every grid slot whose window OVERLAPs a *placed* conflicting exam's real window. Public
  AllowedSlots restricts against ALL placed conflicts (GUI); the solver passes only `placedFixed`
  (movable-vs-movable stays governed by hardSep). Uses exam.MaxDuration (NTA-aware, conservative).
- Hard cross-campus travel buffer: `effectiveGapMinutes(examGap, locA, locB)` returns
  `crossCampusGapMinutes` (=120) when campuses differ (via `locationOf`). Applied in hardSep,
  overlapSlots, validateStudentReg, conflictsFromSlots. NOT yet in spreadcalc (detector, scalar gap
  — noted follow-up).
- validation onlyPlannedByMe now ONLY drops both-foreign pairs (a one-foreign conflict is ours to fix
  by moving our own exam). Behaviour change: false-mode now also shows both-foreign → GUI should
  default the flag to TRUE.

Verified all 3 paths against Test26SS-v2; unit tests in plan_allowedslots_test.go. Committed
da7b88b. crossCampusGapMinutes is now DB-CONFIGURABLE (main bc40186): per-semester config value
(SemesterConfigInputData.crossCampusGapMinutes → semester_config_input → effective
SemesterConfig.crossCampusGapMinutes, default 120); effectiveGapMinutes takes it as a param, all 4
call sites pass p.crossCampusGapMinutes(). GUI-sync: add the field to the semester-config editor.

Display follow-up (31cd5ed): the GUI slot grid queries examsAt per configured slot start, so
off-grid planned exams (external/other-faculty at 11:00, MUC.DAI on :15 raster — 27 in Test26SS-v2)
show in no cell. Added query `examsNotOnSlotGrid: [PlannedExam!]!` (plexams.ExamsNotOnSlotGrid) that
returns planned exams whose Starttime is not a semesterConfig slot start. GUI-SYNC PENDING: GUI must
consume it and render these (recommended: in the slot column whose window they overlap, with the real
HH:MM shown; the exam carries planEntry.starttime + maxDuration). examsAt itself was NOT changed
(exact-match still used by invigilation/rooms/emails). Also: GUI should default validateConflicts
onlyPlannedByMe=true (see above).
