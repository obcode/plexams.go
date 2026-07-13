---
name: roomplan-solver
description: Room planning is now a 4th SA solver (plexams/roomplan); greedy allocator removed; summer heat + turnaround constraints; on feat/roomplan-solver → main.
metadata:
  type: project
---

Raumplanung (room assignment) was rebuilt as the **fourth** simulated-annealing solver, package
`plexams/roomplan`, mirroring `examplan` (immutable `Problem`, mutable `State` implementing
`optimize.Model`, self-describing `Registry()`). The old greedy allocator (`PrepareRoomForExams` +
package `plexams/rooms`) was **deleted** ("replace outright"). Built & validated against Test26SS-v2
on Mongo port 27013 (2026-07-12).

**Key design decisions:**
- Decision variable = **room per single student-seat** (`roomOf []int`), NOT seat-groups with counts.
  Splits / shared rooms emerge from single-seat moves. DB-agnostic package (no `graph/model` import);
  feature/availability/handicap pre-filtered into per-exam `AllowedNormal`/`AllowedAlone`.
- **Room turnaround** is a precise **pairwise** time check (`State.turnaroundConflict`) matching
  `ValidateRoomsTimeDistance` exactly (lag + prev PostExtra + cur PreExtra); an earlier over-block
  and a seat-capacity model were both wrong. EXaHM/SEB 30/30 buffers make back-to-back T3 slots
  mutually exclusive per room — genuine, not a bug.
- **Construct = round-robin across exams in a slot + best-fit room** so two EXaHM/SEB exams co-pack
  the scarce T3 pool instead of one exam filling whole rooms and starving the next.
- **Summer heat (Hitzeschutz)**, active only in SS (or `roomHeatMode=SUMMER`), only own rooms
  (`RequestWith==NONE`; booked ANNY/MANAGEMENT incl. R1.046/049 exempt): `heat-floor` soft (later slot
  → lower floor, floor from `Rx.abc` name or optional `model.Room.Hitzewert` override) + `summer-cooldown`
  hard (own room never in two directly consecutive same-day slots). See [[slot-time-avoidance]].
- 7 hard + 8 soft constraints, shown via new query `roomPlanConstraints` (reuses `OptimizerConstraint`).
- **Room preferences (soft, keep booked T-Bau free for EXaHM):** `seb-rbau` (SEB exam prefers plain
  R-Bau SEB rooms over booked EXaHM rooms) + `exahm-booked` (EXaHM exam incl. room-alone NTA prefers a
  booked T-Bau EXaHM room; an OWN R-Bau EXaHM room like the 1-seat NTA room **R1.011** is only a
  fallback when T3.021 isn't booked). `ValidateRoomsPerExam` emits an INFO hint when that fallback is
  used. Weights `SebAvoidExahm`/`OwnExahmFallback` (internal, not yet in GenerationConfig). With R1.011
  active this took Test26SS-v2 from 52 unplaced to **0** (SEB moves to R-Bau → frees T3 for EXaHM;
  room-alone NTA takes 1-seat R1.011 instead of wasting a 30-seat T3 room).

**GraphQL:** `assignRoomsForExams` repurposed to the solver (now takes `dryRun/seed/iterations/keepAssigned`,
streams `LogLine`, final RESULT carries `RoomPlanReport` with `costByConstraint`/`hardViolations`).
New `GenerationConfig` fields `room*` weights + `roomHeatMode` + `Room.hitzewert`. Refuses to write on
hard violations, else `ReplacePlannedRooms`+`ReplaceUnplacedExams`+`markCondition(condRoomsAssigned)`.

**Follow-ups / notes:** weights are placeholders (`DefaultWeights`) — tune on real data. Reserve rooms
not yet explicitly marked (free-seat buffer emerges as a partly-filled room). SebSeats override not used
(capacity uses physical seats). Pre-existing inconsistencies surfaced (not fixed): `roomcalc.SatisfiesConstraints`
socket check is stricter than `ValidateRoomsPerExam` (socket-only vs socket||lab); `ValidateRoomsNeedRequest`
vs `computeRoomsForSlots` reservation mismatch on MANAGEMENT rooms. Also fixed a latent nil-deref in
`ValidateRoomsPerExam` when a needsRoomAlone NTA is unplaced. GUI-sync pending (see [[gui-and-cli-sync]]).
