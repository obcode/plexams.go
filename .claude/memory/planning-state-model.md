---
name: planning-state-model
description: "Planning workflow as a condition/event Petri net with publish gates that lock generation"
metadata:
  node_type: memory
  type: project
  originSessionId: 6285039b-3933-4bb1-a8f3-24a7355c4a1d
---

The planning workflow (phases -1..3, see [[Ablauf.md in repo root]]) is modelled as a
1-safe condition/event Petri net, decided with Oliver 2026-06-21:

- Declarative net in `plexams/planning_state.go`: phases + milestone **conditions**
  (only milestones + gates, not every step). Extended **in Go code**, not config.
- State (set conditions) stored per semester in DB collection `planning_state`.
- GraphQL: query `planningState` (phases→conditions, `blockedAreas`), mutation
  `setPlanningCondition(key, done)`. GUI shows it on the start page; conditions can be
  toggled by hand, some are set automatically.
- **Auto-computed conditions** (2026-07-12): `planstate.CondDef.Compute func(ctx)(bool,error)`
  makes a condition *derived* — Done recomputed live on every `State()` read, DB value
  ignored, `setPlanningCondition` refuses it, GraphQL field `PlanningCondition.auto=true`
  (GUI renders read-only). Predicates bound per-instance in `p.planningConditions()`
  (called from NewPlexams), since the static `planningConditionDefs` can't close over `p`.
  First one: `otherFKExamsScheduled` ("Alle Prüfungen anderer FKs eingepflegt", first item
  of phase1) = true iff every external + NotPlannedByMe exam has a plan-entry Starttime
  (empty set ⇒ vacuously true, else it'd be unsatisfiable). Phase1 order also fixed so
  "EXaHM/SEB fixiert" precedes "Terminplan generiert".
- **Gates** (inhibitor): `roomPlanPublished` locks generateRoomsForSlots/Exams +
  applyRoomRequestsPreview; `invigilationPlanPublished` locks generateInvigilations
  (via `generationAllowed(area)` in the plexams methods → works for CLI + GUI).
  "Published" = the publish **email** was sent, NOT the ZPA upload.
- Always allowed (never gated): ZPA upload, anny import, all explicit edits
  (block/unblock, pre-plan/rm, change-room, request approve/active, move-to).
  Unsetting a gate condition re-enables generation.
- Auto-marking: operations call `p.markCondition(key)` on success (zpa import,
  prepare connected/generated/studentregs, studentregs upload, rooms generated,
  invig reqs import, invig generated, and the publish emails — emails only when
  `run==true`).
- After an anny import, `ValidateRoomsForSlotsFresh` runs so a stale rooms-for-slots
  cache is visible immediately.
- **Send-once emails**: most workflow emails may be sent only once. `emailSendAllowed(ctx,
  condKey, run)` refuses run=true while the condition is set (dry-run always ok); resend by
  unsetting. Gated: exahm, constraints, prepared, draft, published-exams/rooms/invigilations,
  room-requests, invigilations(request), cover-pages-to-all (coverPagesSent = last step of
  phase 3). NOT gated (repeatable): primuss-data (all/single/unplanned), single cover page,
  NTA mails (new-nta/room-alone/planned), invigilations-missing. The GUI should offer only
  dry-run once the condition is set. All workflow emails are now in the GUI as subscriptions.

See [[room-requests]], [[gui-and-cli-sync]].
