---
name: plexams-decomposition
description: "Marathon to decompose the plexams god-package into service packages; strategy = net-first, then peel concerns"
metadata:
  node_type: memory
  type: project
  originSessionId: 6285039b-3933-4bb1-a8f3-24a7355c4a1d
---

Goal: break the ~24k-LOC `plexams` god-package into service packages so email/pdf orchestration depends on small focused interfaces (see [[cli-to-gui-migration]]). Chosen approach: "erst Domäne zerlegen" before removing `plexams/email_*.go`.

Key finding (2026-07): **there is no clean leaf.** Every concern is coupled both via `*Plexams` AND woven across files. Even `anny` (smallest outbound: db+config) has ~12 inbound callers and its core functions are spilled across files: `annyBookedBySlot` (preplan_booking.go, feeds the generator), `ExahmRoomsFromAnnyBookings` (rooms.go), `maxNonAnnySebRoom` (preplan_overview.go). Output concerns (pdf/email) are the inverse: low inbound, ~30 outbound domain calls → big interfaces.

Strategy: per-concern slice = (1) characterization tests around the area, (2) untangle spilled funcs + extract to package with a small `deps` interface `*Plexams` satisfies, (3) rewire callers to `p.<svc>.X`, (4) semilinear merge (`--no-ff` + push, see [[git-workflow]]). Order: shared booking/room logic first (riskiest, feeds generator), output layers (pdf/email) last.

**How to apply:** Status — Slice 0 DONE (19151a2): char. net for anny/room booking logic. Slice 1 DONE: `plexams/anny` extracted. Also: renamed the anny-fed `BookedEntry`→`AnnyRoomBooking` + dropped dead booked code; a dead-code sweep (355 lines: ChangeRoom, GetExamsForStudent, InvigilationTodos, commented blocks). Slice 2 DONE (55c0d56): `plexams/primuss` — the Primuss XLSX import subsystem (tiny DB iface RawCollection/ReplaceRawCollection, pure parsing; delegators ImportPrimussZip/Dir; orchestration HTTPUploadPrimussZip/affectedZpaAncodes + the thin query/fixData db-passthroughs stayed in plexams).

Pattern proven twice: extract the LOGIC (integration/parsing) to a pkg with a small deps interface + keep facade delegators = zero API/CLI/GUI change. Leave thin 1-line db-passthroughs in plexams (moving them is indirection without value). **anny was the only truly clean leaf; remaining slices all have a catch** (measured): primuss/zpa need bigger deps ifaces + cross-domain hooks; planning_state has tiny outbound but 53 call-sites use its cond* constants (churn or partial engine-extraction); zpa_post couples to plan/invig. Next candidates: zpa, planning_state, preplan, invigilation, rooms. One concern per branch.
