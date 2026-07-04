---
name: graphql-interface-cleanup
description: GraphQL schema was audited vs GUI usage and fully trimmed (unused queries+mutations removed, 3 validators wired into GUI). Done.
metadata:
  node_type: memory
  type: project
  originSessionId: da8a958a-7cc3-4731-88bb-90c5303bc30c
---

The GraphQL API was audited against actual plexams.gui usage (2026-07) and the unused surface was removed on `main` via fast-forward (linear history, commits `9f7a2ec`, `c8a97f4`, `d56ef6c`).

Removed: all 10 GUI-unused **Mutations** (addExamToSlot stub; excludeDays/possibleDays/sameSlot/placesWithSockets/rmConstraints superseded by `addConstraints`; zpaExamsToPlan; the 3 one-off config→DB migrations incl. CLI `invigilation migrate-constraints`) and all 32 GUI-unused **Queries**, together with the now-dead `*Plexams`/`db` methods behind them (kept anything with a live internal/CLI/test caller — e.g. `GetZpaExamsToPlan`, `RmConstraints`, `db.PreplanExam`).

The 3 backend-complete validators that were the only "wire-up" item — `validateStudentRegs`, `validateDB`, `validatePrePlannedExahmRooms` (parameterless `LogLine!` subscriptions ending with a `ValidationReport`) — have since been wired into the GUI. Audit fully closed. See [[gui-and-cli-sync]].
