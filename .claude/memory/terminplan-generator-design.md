---
name: terminplan-generator-design
description: Design for the exam-schedule (Terminplan) generator and shared SA optimizer core; the big next feature.
metadata:
  node_type: memory
  type: project
  originSessionId: 6285039b-3933-4bb1-a8f3-24a7355c4a1d
---

Terminplan (exam schedule) generation — design settled 2026-07-02.

STATUS (2026-07-02): IMPLEMENTED — generic `plexams/optimize` SA core; `plexams/examplan` solver (incremental cost ~1M iters/3s; calibrated DefaultWeights Adjacent 2500/SameDay 900/WorstCase 0.05); builder `buildExamPlanProblem` + `GenerateExamSchedule` (reporter/streaming, writes non-locked entries, gated); GraphQL subscription `generateExamSchedule` + queries `examScheduleConstraints`/`examScheduleConflicts` + conflict-rating CRUD (`setConflictRating`/`setExamsCanShareSlot`/`canShareSlotSuggestions`); planning-state `examScheduleGenerated` + EXAMS gate. STILL TODO: steps ③④ port pre-plan & invigilation onto the generic core; small follow-up: expose affected-student mtknrs per conflict for per-student ACCEPTED rating; GUI-side diff of successive conflict lists.

Original agreed design below.

**Architecture:** extract a GENERIC optimization core `plexams/optimize` (simulated annealing: geometric cooling, Metropolis, hard=move-veto/soft=weighted Registry, incremental cost deltas) used by all three solvers. Build order: ① core → ② Terminplan in `plexams/examplan` (the feature) → ③ port the pre-plan solver onto it (its twin) → ④ port the invigilation optimizer (`plexams/invigplan`) → ⑤ GraphQL/GUI/planning-state. The pre-plan solver ([[preplanning-seb-exahm]]) is essentially "Terminplan for a subset" and is the closest analog.

**Unit to schedule:** AssembledExam that is toPlan, not Locked, not external/notPlannedByMe. Domain = AllowedSlots(ancode). Fixed obstacles (untouched, but count for conflicts): Locked plan entries, external + notPlannedByMe (times set via GUI).

**Hard constraints:** fixed placements untouched · SameSlot groups together · no student two exams in same slot (baseline hard, lifted ONLY by pair canShareSlot) · EXaHM per slot ≤ booked EXaHM Anny seats · global seats per slot ≤ total room capacity · allowed slots per exam · "unzulässig"-rated pairs hard-separated else infeasibility report.

**Soft objective (tiers high→low):** ① spread (primary): distance = real time between slot Starttimes (weekends auto-farther); per (student, pair) decreasing penalty; **sum + convex worst-case-per-student**; ladder same-slot(hard)/adjacent-same-day(very high)/same-day(high)/consecutive-days(medium, then ~1/Δ); repeater discount via IsRepeaterExam flag OR group-vs-Groups semester heuristic. ② unerwünscht pairs boosted. ③ section clustering: same module+program, different examer → pull together (canShareSlot removes their conflict; auto-suggest+confirm like [[mucdai-import-linking]]). ④ small-exam (≤5 regs) same-examer → same slot. ⑤ slot-load: convex penalty on total seats/rooms per slot (two 100-exams never together). ⑥ SEB Anny preference (overflow to R allowed).

**Conflict rating (DB, replaces YAML knownConflicts; GUI loop):** two acceptance mechanisms of DIFFERENT strength — pair-level `canShareSlot` (structural, on ZPA/external exam constraints, mirrors pre-plan; removes pair from conflict graph incl. lifting the hard same-slot ban) vs. per-student acceptance `(mtknr, A, B)` (zeroes only that student's SOFT proximity term; same-slot stays HARD for him). Pair ratings 3-level: akzeptiert/unerwünscht/unzulässig (+default). Each regeneration recomputes actual conflicts → show diff (gone/stayed/new) + flag stored ratings "no longer relevant"; option to regenerate ignoring ratings (kept stored).

**Defaults:** warm start both from-scratch and keepAssigned; seed=1 deterministic; adjacency only within a day; NTA time-overrun makes the next slot a hard conflict for affected students; weights calibrated so spread dominates, clustering is a tie-breaker.

**Constraint introspection (read-only):** constraints in the generic core are self-describing (Name/Title/Description + kind hard/soft + weight/tier); a read-only GraphQL query lists, per generator (Terminplan/Pre-Plan/Aufsichten), the currently-applied hard & soft constraints so the user can see what is considered. No configuration via GUI, only understanding.

**Surface:** streaming subscription `generateExamSchedule(dryRun, seed, iterations)` (like AssignInvigilations); writes only non-locked PlanEntries; validate via existing ValidateConflicts; new phase-1 planning-state point "Terminplan generiert" + gate (see [[planning-state-model]]). Reuse: ValidateConflicts already does the per-student same-slot/adjacent/same-day pairwise detection.
