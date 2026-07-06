---
name: slot-time-avoidance
description: Terminplan soft constraint avoiding early (WS) / late (SS) start times; AUTO-by-semester toggle in GenerationConfig; T-Bau phase-A exception.
metadata:
  node_type: memory
  type: project
  originSessionId: 45634ece-9b26-4445-8ed6-d852d423160d
---

Soft constraint in the Terminplan SA solver ([[terminplan-generator-design]]) that weights exam
**start times** (Beginn-Uhrzeit, not slot number), semester-dependent:
- **Winter**: threshold — avoid starts before ~10:00 (08:30). Later slots all equally fine.
- **Summer**: monotonic — the later the start, the worse, so **earlier is always better**;
  size-weighted (per registration) so LARGE exams get pulled to the front. (Replaced the older
  "avoid after 13:00" threshold on 2026-07-06 per user: "große Prüfungen früh besser als spät".)

**Why:** early slots in winter are unpopular; in summer afternoons are hot, and a big exam early
spares more students. User plans with ~15 instead of 10 days to leave slack.

**How it works (backend, on `main` as of 2026-07-06):**
- `examplan`: `Weights.TimeOfDay` + `Problem.TimeSeverity` (per-slot severity, hours of badness)
  + `timePenalty(seats,slot)=TimeOfDay*severity*seats`; incremental `timeTotal` in `State`; soft
  constraint `timeOfDayC` in `solve.go`. Per-seat weighting = student impact.
- `plexams/examplan_time.go`: `slotTimeSeverity` (config→severity+weight) + pure
  `computeSlotTimeSeverity` (winter=threshold, summer=monotonic from earliest start);
  `resolveSlotTimeMode` (AUTO follows semester via `isSummerSemester` = suffix "SS"); wired in
  `buildExamPlanProblem`.
- Config in the **global** `GenerationConfig` (mutation `setGenerationConfig`): `slotTimeMode`
  (enum AUTO/WINTER/SUMMER/OFF, default AUTO), `slotTimeWeight` (default **2.0**, per reg/hour),
  `slotTimeWinterEarliest` ("10:00", winter only). Backfilled for older stored configs.

**T-Bau exception (phase A = `GenerateExamRoomsPhase`, EXaHM/SEB into booked Anny/T-Bau
slots):** summer/off → penalty fully OFF (climate-controlled, go by booking); winter → only a
gentle pull (weight × `tbauSlotTimePullFactor`=0.4) toward later starts so an 08:30 booking is
left empty when possible (user may drop it for R-rooms). Phase B applies the full penalty.

**Debugging note:** `runExamGeneration` now logs/streams each concrete hard violation (not just
the count) — "refusing to write: N hard violations" no longer hides *which*. A `From`-date
change reindexes day numbers; stale plan entries / phaseFixed / locked exams then clash (esp.
EXaHM capacity, since Anny bookings map by date). Recover via unfix → reset → regenerate.

**How to apply:** default weight is a guess — tune `slotTimeWeight` via dry-run generation.
GUI ([[gui-and-cli-sync]]): generation-config page needs a mode dropdown, a weight number, and
one HH:MM field (winter earliest). No CLI command touches exam-schedule generation.
Related: [[two-phase-exahm-seb]].
