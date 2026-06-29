---
name: room-requests
description: "How building-management room requests work in plexams.go (generate/apply/email workflow + conventions)"
metadata:
  node_type: memory
  type: project
  originSessionId: 6285039b-3933-4bb1-a8f3-24a7355c4a1d
---

Building-management room requests (Gebäudemanagement) in plexams.go, decided with Oliver 2026-06-21:

- Request rooms split by **how** they are requested: `Room.requestWith` enum NONE/ANNY/MANAGEMENT.
  ANNY = T-building rooms (Anny website); MANAGEMENT = via Gebäudemanagement (these go through the
  request-email flow). `needsRequest` is derived (requestWith != NONE).
- `Room.requestPriority`: lower = preferred when generating. Convention this semester:
  R1.006 + R1.046 = priority 1, R1.049 = priority 2 (R1.049 only added when 1 doesn't cover).
- Requests live in per-semester DB collection `room_requests` (key room/day/slot), NOT in yaml
  anymore. Migrated once from `roomConstraints.<room>.reservations` via migrateRoomRequestsFromConfig
  (this semester: 48 imported, 34 approved). `booked`/Anny in yaml is ignored.
- **Workflow**: generate once (read-only `roomRequestsPreview`) → eyeball in GUI → apply once
  (`applyRoomRequestsPreview`, replace-all, NO merge; refuses to overwrite existing unless force) →
  approve/deactivate per request → send email. Never re-generate within a running semester. Extra
  rooms via `addRoomRequest`; extend a request for NTA via `updateRoomRequestTime`.
- Stored from/until include **±15 min buffer** (setup/teardown). Email goes to
  `semesterConfig.emails.roomManagement` via `sendEmailRoomRequests` (dry run → testmail).

See [[emails-over-graphql]], [[gui-and-cli-sync]], [[cli-to-gui-migration]].
