---
name: slotless-timebased-redesign
description: "Planned big refactor — drop slot/day numbers entirely, store exams as absolute times; conflicts/rooms/invigilation become interval-based."
metadata:
  node_type: memory
  type: project
  originSessionId: 70930680-8219-4f0b-b5a8-65a5b770bd06
---

Decided direction (planner Oliver, 2026-07): replace the ordinal
`(DayNumber, SlotNumber)` placement model with **absolute times only**. Motivation:
changing the exam period today shifts the meaning of every stored day/slot number.

Key decisions:
- **Full slot elimination**, not the conservative "keep slots internal" variant.
- **No data migration** — clean cut starting with a new semester.
- **CLI is being removed** ([[cli-to-gui-migration]]) → delete slot-related CLI
  parts (cmd/plan.go etc.) instead of adapting them.
- Conflicts: time-distance rule (overlap / gap < ExamGapMinutes / "too close")
  instead of sameSlot/adjacent — more correct (duration + NTA aware).
- Rooms & invigilation: interval + turnaround buffer (`GenerationConfig.TimelagMin`
  already exists for invigilations) instead of slot buckets + "next slot blocked".
- Config `Slots []string` reinterpreted as "allowed start times"; `MucDaiSlots
  [][]int` → absolute times.

Staging (representation ≠ behaviour): **Stufe 1** = time representation at the SAME
granularity as today's slots (golden-testable "same input → equivalent plan");
**Stufe 2** (later) = finer/free start times + room capacity in the solver.

Goal this serves: summer semester should fit all exams into mornings (see
[[slot-time-avoidance]]) — finer time granularity gives the solver more room.

Detailed file-by-file plan + GUI change list: `docs/plan-slotless-timebased.md`
in the repo. Remember [[gui-and-cli-sync]] — needs plexams.gui-agent instructions.
