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

Slice 3 DONE (1ecae53): `plexams/planstate` — the generic condition/event Petri-net ENGINE (Machine over a 2-method DB iface). Solved the cond*-constant churn by **injection**: the concrete net (cond* keys + phase/cond defs) stays declared in plexams and is passed to `planstate.New(db, phases, conds)`; delegators keep identical signatures → zero call-site churn. Engine now unit-testable with a fake DB (no mongo).

Patterns proven: (a) extract the LOGIC to a pkg with a small deps interface + facade delegators = zero API/CLI/GUI change; (b) when many call-sites share constants/config, **inject the policy** and move only the mechanism = zero churn; (c) leave thin 1-line db-passthroughs in plexams. **anny was the only truly clean leaf; the rest need one of these techniques.** Remaining: zpa (zpa_post couples to plan/invig — extract the fetch/import like primuss, leave upload orchestration; note name clash with the existing top-level `zpa` client pkg → use a different pkg name), preplan, invigilation (solver already in invigplan; consider extracting a pure core like fairInvigilationTargets), rooms. One concern per branch.
