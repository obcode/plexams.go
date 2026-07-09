---
name: preplan-compaction-overflow
description: Preplan solver now packs disjoint-program exams into fewest slots (cancel bookings) + oversized SEB placed with R-Bau overflow warning.
metadata:
  node_type: memory
  type: project
  originSessionId: 83ce610f-8017-4495-b5b3-1e8e17c9bd24
---

Two additions to the pre-plan solver (plexams/preplan_solve.go, preplan_assign.go), on main after the 2026-WS analysis (2026-07-09):

**Compaction (Task B):** goal of Vorplanung is also to see how many Anny bookings can be *cancelled*. Added `preplanSlotOpenCost` (=15) charged once per occupied slot in the SA cost, plus best-fit + prefer-already-open tiebreak in `chooseSlot`. Exams with DISJOINT study programs (empty intersection) are now consolidated into the same slot (run simultaneously); same-program exams still spread (ideally across days). Weight rationale: merging two same-program exams flips their spreading penalty from ≤75 (adjacent 2h slots, `preplanProgramConflictWeight*(8-2)/8`) to 100 (same start) = +25 swing, so `preplanSlotOpenCost` MUST stay < 25 or same-program exams get merged (breaks TestSolvePreplanSeparatesSameProgramBySlot). New INFO finding lists cancellable (unused booked) slots via `cancellableSlotsFinding`.

**Oversized SEB overflow (Task A):** an SEB bigger than any single booked slot's covering window (e.g. Datenbanksysteme 160 vs 120 max per slot = 4 rooms × 30) used to be dropped. Now, if the overflow beyond the fullest slot fits the R-building (`<= rBauSebThreshold`), it's PLACED filling one Anny slot; `preplanUnit.rBauOverflow` records the remainder and `validatePreplan` emits a WARNING "zusätzlich N Plätze im R-Bau einplanen" (not an error). Anny seat demand is capped via `preplanAnnyDemand` in all three validation checks (window, per-slot sum, cumulative). EXaHM never overflows (needs EXaHM rooms).

2026-WS result: was ~11 scattered slots → 7 slots, 21/28 bookings cancellable. Only genuine remaining unplaced = a pure MUC.DAI EXaHM (Softwareentwicklung [DE GS ID]) because 0 MUC.DAI slots are configured — that is a user/config error, not a solver issue (any exam touching DE/GS/ID is locked to MUC.DAI slots; see [[preplanning-seb-exahm]]). No GraphQL schema change; findings render via existing PreplanValidation. See also [[preplan-validation-severity]].
