---
name: slot-time-avoidance
description: Terminplan soft constraint avoiding early (WS) / late (SS) start times; AUTO-by-semester toggle in GenerationConfig; T-Bau phase-A exception.
metadata:
  node_type: memory
  type: project
  originSessionId: 45634ece-9b26-4445-8ed6-d852d423160d
---

Soft constraint in the Terminplan SA solver ([[terminplan-generator-design]]) that avoids
unfavourable exam **start times** (Beginn-Uhrzeit, not slot number): winter → avoid starts
before ~10:00 (08:30); summer → avoid starts after ~13:00 (14:30/16:30, later = worse).

**Why:** early slots in winter and late slots in summer are undesirable; user plans with ~15
instead of 10 days to leave slack to avoid them.

**How it works (backend, done, on `main`? no — uncommitted as of 2026-07-06):**
- `examplan`: new `Weights.TimeOfDay` + `Problem.TimeSeverity` (per-slot hours outside the
  window) + `timePenalty(seats,slot)=TimeOfDay*severity*seats`; incremental total `timeTotal`
  in `State`; soft constraint `timeOfDayC` in `solve.go` Registry. Penalty scales with
  registrations (student impact).
- `plexams/examplan_time.go`: `slotTimeSeverity` (config→severity+weight) and pure
  `computeSlotTimeSeverity`; `resolveSlotTimeMode` (AUTO follows semester via
  `isSummerSemester` = suffix "SS"); wired in `buildExamPlanProblem`.
- Config lives in the **global** `GenerationConfig` (mutation `setGenerationConfig`): new
  fields `slotTimeMode` (enum AUTO/WINTER/SUMMER/OFF, default AUTO), `slotTimeWeight`
  (default 5.0, per reg/hour), `slotTimeWinterEarliest` ("10:00"), `slotTimeSummerLatest`
  ("13:00"). Backfilled for older stored configs.

**T-Bau exception (phase A = `GenerateExamRoomsPhase`, EXaHM/SEB into booked Anny/T-Bau
slots):** summer/off → penalty fully OFF (climate-controlled, go by booking); winter → only a
gentle pull (weight × `tbauSlotTimePullFactor`=0.4) toward later starts so an 08:30 booking is
left empty when possible (user may drop it for R-rooms). Phase B applies the full penalty.

**How to apply:** defaults are guesses — tune `slotTimeWeight` via dry-run generation.
GUI ([[gui-and-cli-sync]]): add controls on the generation-config page for the 4 fields
(mode dropdown + weight number + two HH:MM fields). No CLI command touches exam-schedule
generation. Related: [[two-phase-exahm-seb]].
