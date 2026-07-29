---
name: preplan-booking-proposal
description: preplanBookingSuggestions runs the pre-plan solver on the still-FREE Anny rooms and derives which rooms to book; read-only, no persistence.
metadata:
  node_type: memory
  type: project
---

Added 2026-07-29 (commit 97b19c8, on main). The pre-plan assignment can only distribute
what is ALREADY booked in Anny; when an exam stayed unplaced the only advice was "book more
Anny slots". The new query answers the inverse question:

```graphql
preplanBookingSuggestions(keepAssigned: Boolean! = true): PreplanBookingProposal!
```

Pipeline in `plexams/preplan_suggest.go`:

1. `freeAnnyIntervals` = per Anny room and exam day, the complement of the **foreign**
   confirmed bookings (`subtractWindows`). Our own bookings do NOT block — a room we hold is
   available to us, and it may be extendable beyond our window. Foreign = personalization
   name not in `annyConfig.personalizationNames` (same rule as `AnnyBooking.mine`).
2. `solvePreplanAssignment` runs on that capacity. Slot starts we already hold bookings for
   are flagged `preplanSlot.alreadyBooked` → they cost no `preplanSlotOpenCost` and win the
   `chooseSlot` tiebreak, so the proposal prefers slots that need no new booking. Without
   this the "komplett neu" mode proposed 32 bookings where 8 suffice (live Test26SS-v2).
3. `roomsToBookForSlot` derives, per occupied slot, the union exam window (duration +
   `exahmRoomBuffers`) and greedily covers it with the largest free rooms — EXaHM demand only
   from EXaHM-capable rooms, honouring per-exam `allowedRooms` via `preplancalc.RoomsForKind`.
   Rooms we already booked for that window count first and are never proposed.
4. `mergeBookingSuggestions` merges adjacent windows of the same room into one booking.

Nothing is persisted — neither the pre-exams nor Anny (the Anny token stays read-only).
`keepAssigned=true` pins the already-slotted exams (minimal addition, the "where do I still
book?" case), `false` re-plans everything (the "nothing booked yet" case).

Enabling this required extracting the solver core out of `GeneratePreplanAssignment` into
`solvePreplanAssignment` (non-persisting, capacity injected as `preplanCapacity`). The real
assignment never sets `alreadyBooked`, so its behaviour is unchanged — the existing
`preplan_solve_test.go` expectations still hold.

See [[preplanning-seb-exahm]] for the whole pre-planning feature, [[preplan-compaction-overflow]]
for why `preplanSlotOpenCost` must stay < 25, and [[exahm-time-window-coverage]] for the
window/buffer rules the proposal reuses.
