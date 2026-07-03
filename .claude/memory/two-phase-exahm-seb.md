---
name: two-phase-exahm-seb
description: "Terminplan generation splits into phase A (EXaHM/SEB into booked T-Bau slots) then phase B (rest), with a separate PhaseFixed freeze."
metadata:
  node_type: memory
  type: project
  originSessionId: 6285039b-3933-4bb1-a8f3-24a7355c4a1d
---

Exam-schedule (Terminplan) generation is two separate phases, reusing the single `plexams/examplan` SA solver (no second solver — the user explicitly rejected duplicated code).

- **Phase A** = only EXaHM/SEB exams movable; objective MAXIMIZE T-Bau room utilization (weight `TbauFill` high, `SlotLoad` off). `TbauFill` penalizes unused-booked seats per slot → drives EXaHM/SEB into slots that actually have booked T-Bau rooms; EXaHM hard-capped, SEB uncapped (partial overflow SEB→normal rooms preferred over all-normal). Booked seats come from `annyBookedBySlot` filtered by personalization names (only MY bookings count).
- **Freeze** = `FixExamRoomsPhase` sets `PlanEntry.PhaseFixed` (distinct from the manual `Locked`) on every EXaHM/SEB exam that has a plan entry; `UnfixExamRoomsPhase` clears all. This is a *different* freeze than the user's explicit Locked.
- **Phase B** = regular `GenerateExamSchedule`; PhaseFixed entries are fixed obstacles (untouched).

Backend: `runExamGeneration(ctx, roomPhase, dryRun, seed, iterations, ignoreRatings, reporter, doneCond)` serves both; `buildExamPlanProblem(ctx, applyRatings, roomPhase)`. Planning-state points (phase1): `condExahmSebPlanned` "EXaHM/SEB in T-Bau-Räume geplant", `condExahmSebFixed` "EXaHM/SEB fixiert (für Phase 2)".

GraphQL: subscription `generateExamRoomsPhase(dryRun,seed,iterations)`, mutations `fixExamRoomsPhase: Int!` / `unfixExamRoomsPhase: Boolean!`. Generator is GUI-only (no CLI command). Commits efbe7c7 / f2bd707 / 9c4c1d9.

Verified END-TO-END live on Test26SS (real writes, snapshot+restore): Phase A → fixExamRoomsPhase → Phase B; Phase A 24 placed/0 unplaced/0 hard, Phase B 82 placed/0 unplaced/0 hard, PhaseFixed respected 28/28 (0 moved). TbauFill weight 10000 confirmed (all EXaHM/SEB in booked T-Bau slots; used<booked only because supply>demand — not a calibration issue). See [[planning-state-model]], [[preplanning-seb-exahm]], [[room-requests]].
