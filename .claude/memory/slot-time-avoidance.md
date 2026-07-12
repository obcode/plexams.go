---
name: slot-time-avoidance
description: Terminplan start-time window — winter no-early / summer no-late, HARD-by-default (domain restriction) with SOFT override; EXaHM/SEB exempt.
metadata:
  node_type: memory
  type: project
  originSessionId: 45634ece-9b26-4445-8ed6-d852d423160d
---

Semester-dependent **start-time window** in the Terminplan SA solver ([[terminplan-generator-design]]),
weighting exam **start times** (Beginn-Uhrzeit). Recalibrated 2026-07-12 from the old soft-only design
to a hard-by-default window:

- **Winter**: exams must **not start before** a limit (default **10:00**, `slotTimeWinterEarliest`).
- **Summer**: exams must **not start after** a limit (default **14:00**, `slotTimeSummerLatest`) —
  non-climatised rooms get too hot in the afternoon — **plus** a mild "earlier is better" gradient
  below the cutoff (`slotTimeGradientWeight`, default 2.0), size-weighted so LARGE exams go first.
  NB: the summer rule is on the **start time** (user chose start ≤ cutoff, NOT end-of-duration), so it
  stays per-slot / duration-independent.

**Enforcement (`slotTimeEnforcement`, default HARD):**
- **HARD** = domain restriction (like MUC.DAI / EXaHM-window): non-exempt exams get out-of-window slots
  removed from `Allowed`; one that fits nowhere is left **UNPLACED with a reason** (rest of plan still
  written) — does NOT block the whole write. Weaken to SOFT for a deliberate emergency deviation.
- **SOFT** = strong penalty (`slotTimeWeight`, default **20000**, per reg/hour outside; below Unplaced
  1e6): exam may sit outside but is reported as a `time-of-day` violation ("beginnt vor 10:00" / "nach 14:00").

**Exemption:** EXaHM/SEB exams (booked, climate-controlled T-Bau rooms) are exempt from BOTH the domain
filter and the soft penalty — `Problem.timeExempt(u) = Units[u].Exahm||Seb`. Phase A (`GenerateExamRoomsPhase`)
schedules only EXaHM/SEB, so the window is inert there. The old `tbauSlotTimePullFactor` / roomPhase
severity branch was **removed**.

**Where (backend, main as of 2026-07-12):**
- `plexams/examplan_time.go`: `slotTimeSpec` (config→spec) + pure `computeSlotTimeSpec`
  (windowHours + gradientHours; HARD→forbidden[]+gradient severity, SOFT→folded severity). Replaced
  `slotTimeSeverity`/`computeSlotTimeSeverity`.
- `plexams/examplan_build.go`: HARD domain filter over movable non-EXaHM/SEB units (mirrors the
  EXaHM-window block); reason `unplaceableOutsideTimeWindow`; `SetTimeSeverity`+`SetTimeWindow`.
- `plexams/examplan/problem.go`: `TimeWindowMode` + window fields + `SetTimeWindow`, `timeExempt`,
  `outsideWindow`, `windowBreachMessage`; `timePenalty(u,slot)` now per-unit (exempt-aware).
- `graph/generation_config.graphqls`: enum `SlotTimeConstraintEnforcement{HARD,SOFT}` + fields
  `slotTimeEnforcement`, `slotTimeSummerLatest`, `slotTimeGradientWeight`. Defaults/backfill in
  `plexams/generation_config.go` (`fillSlotTimeDefaults`, `defaultGenerationConfig`).

**How to apply:** weights need tuning against real data (Test26SS) — the HARD default means
`slotTimeWeight` only matters in SOFT mode. GUI ([[gui-and-cli-sync]]): generation-config page needs a
HARD/SOFT select, a second HH:MM field (summer latest), and a gradient-weight number; document that
summer is now a start-cutoff and `slotTimeWeight` is SOFT-only. Related: [[two-phase-exahm-seb]].
