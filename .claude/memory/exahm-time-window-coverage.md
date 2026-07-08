---
name: exahm-time-window-coverage
description: "EXaHM/SEB placement gated by REAL Anny booking window (duration + 30/30 buffer, override per exam); Vorplanung + Terminplanung Phase 1 + room display; on main."
metadata:
  node_type: memory
  type: project
  originSessionId: 70930680-8219-4f0b-b5a8-65a5b770bd06
---

Done 2026-07-08 (branch feat/exahm-time-windows merged to main @ cb9476b). Fixes Issue 2 from the Test26SS-v2 durchspielen: "Embedded Computing" (120 min + 60/60 buffer) was planned 21.07 16:30 into a room booked only 16:00–18:30, and T-Bau rooms showed "free" at 14:30.

**Model** ([[preplanning-seb-exahm]], [[two-phase-exahm-seb]]): an EXaHM exam may only sit where a booked EXaHM room's Anny interval fully covers `[start-pre, start+dur+post]`. Default buffer 30 min each side (`exahmDefaultBuffer`), SHARED 15/15 between two of our own consecutive exams in one room (so a 14:00–18:30 booking fits two 90-min exams at 14:30 & 16:30), full 30 to a booking edge/foreign exam. Overridable per exam via `RoomConstraints.PreExamMinutes/PostExamMinutes` — replaces the default (may widen to real 60 for a lab exam, or shorten to 15). SEB is NOT gated (may overflow to R-Bau).

**Core** `plexams/exahm_intervals.go` (NB: NOT `_windows.go` — that suffix = Go GOOS=windows build constraint, file silently ignored!): `exahmRoomBuffers`, `bookedExahmIntervals` (booked T-Bau rooms as merged time intervals w/ EXaHM/SEB caps via anny.MergeRoomBookings), `exahmWindowSeats` (SEAT-accurate: sums seats of booked rooms whose window fully covers the exam) + `exahmWindowCovered` wrapper, `preplanExamDuration`, `intersectSlotSet`. Tested against the user's exact examples.

**Follow-up fixes (2026-07-08, merged @ 99e6e8d):** (1) gate is now SEAT-accurate — a slot is allowed only if rooms COVERING the window provide ≥ students (not just "some room covers it"); catches Softwareentwicklung I (EXaHM 40) placed 16.07 10:30 while T3.023/T3.021 booked only to 11:30 (block-based seat total looked fine). Both preplan+examplan gates and `validatePreplan` use `exahmWindowSeats >= seats`. (2) small SEB (≤ R-Bau threshold) no longer hard-excluded — they fill leftover Anny capacity via low drop cost `preplanSmallSebDrop`=2000 (yields to must-place exams; else R-Bau note). NB user actively commits to main in parallel (e.g. ZPA persons import) — rebase/merge, don't clobber their uncommitted files.

**Wiring**: Vorplanung (`preplan_assign.go`) restricts each EXaHM `preplanUnit.allowedSlots` (empty map ⇒ dropped); `validatePreplan` flags any placed EXaHM exam no booking can cover (gained `exahmIntervals`+`blockDur` params). Terminplanung (`examplan_build.go`) intersects each EXaHM `Unit.Allowed` with window-covered slots using the `[]int{-1}` unplaceable sentinel (same trick as MUC.DAI); overrun `turnaround` now = `exahmDefaultBuffer` (30) so default EXaHM exams don't overrun, only widened Nachlauf does. Display (`rooms_for_slots.go`): booked room offered in a slot only if booking covers the whole slot block (matches `annyBookedByTime`), and an ANNY room whose booking covers no slot is IGNORED not dumped into all slots (root cause of "14:30 frei").

**No GraphQL/schema change** (Pre/PostExamMinutes already in RoomConstraintsInput + setPreplanExamConstraints). Consequence to expect while testing: existing bookings sized for the old (no-buffer) rule may now surface new "Fenster nicht abgedeckt" findings — correct feedback to book 30 min wider.

**GUI**: expose roomConstraints.preExamMinutes/postExamMinutes (Vorlauf/Nachlauf min, default 30) in the preplan-exam constraints form so Embedded Computing can be set to 60/60.

Still open: Issue 3 (informational — small movable exams parallel to EXaHM could be relocated; solver calibration, low prio).
