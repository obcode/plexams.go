---
name: phase-a-overflow-fix
description: Phase A (EXaHM/SEB) now keeps big exams in T-Bau bookings via a per-seat R-Bau overflow penalty + booking-aware greedy order
metadata:
  node_type: memory
  type: project
  originSessionId: cb67cae5-5daf-4ee8-8076-aa8d3ba43aa3
---

Fixed 2026-07-12 (main 450781c). Symptom: in Terminplan Phase A ([[two-phase-exahm-seb]]) a large
SEB exam (355, 84 regs) landed on 06.07 08:30 where there was NO T-Bau booking, while several
fitting SEB bookings (16.–24.07) sat empty. Seed-dependent (some seeds placed it fine).

Root cause: R-Bau overflow was FREE in the objective — the only room term (tbauFillCost) penalized
UNUSED booked seats, never seats placed BEYOND a booking. So an overflow-prone exam had no gradient
back to the bookings. Made worse by the greedy constructor: addedCost ignored bookings entirely and
ordered units by fewest-feasible-slots, so a big exam (many empty slots feasible) was placed late,
after small exams grabbed the bookings; 355 also only allowed 5 of 15 booked SEB slots (its
allowedSlotsFor domain, tightened by [[sameslot-offgrid-conflict-fix]]).

Fix (soft, per user choice): new Weights.OverflowSeat (per seat placed beyond a slot's booked
EXaHM/SEB capacity), set to 10000 in the roomPhase branch of buildExamPlanProblem (0 in phase B →
phase B byte-for-byte unchanged). Because it is per-seat, spilling a big exam costs far more than a
small one → big exams stay in T-Bau, small ones overflow to R-Bau. Also: overflowC soft constraint
(model.go overflowCost / problem.go overflowPenalty), marginal overflow folded into the greedy
addedCost, and in phase A the greedy orders by fewest BOOKED-slot options first (bookedFeasCount) then
larger (moreConstrainedForPhase). Verified 355 → booked slot across seeds 1/7/42/100/2024; unit tests
in plexams/examplan/overflow_test.go. OverflowSeat=10000 is a calibration constant (could be tuned).
