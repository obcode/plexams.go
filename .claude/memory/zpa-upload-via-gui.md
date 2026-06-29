---
name: zpa-upload-via-gui
description: ZPA plan upload happens via the GUI going forward; errors must surface through the streaming reporter.
metadata:
  node_type: memory
  type: feedback
  originSessionId: 6285039b-3933-4bb1-a8f3-24a7355c4a1d
---

The user uploads the exam plan to ZPA via the GUI from now on, not the CLI
(`zpa upload-plan`). The GUI uses the streaming subscriptions
`uploadExamsToZPA` / `uploadExamsWithRoomsToZPA` / `uploadExamsWithInvigilatorsToZPA`,
which call `Plexams.UploadPlan` through `runExclusiveOp`.

**Why:** When `UploadPlan` returns an error, `runExclusiveOp` emits it as a
`LogLevelError` line in the stream, so the GUI shows it. Anything that should be
visible to the user on upload must therefore come back as a returned error or a
`reporter.Warnf`/`Println` line — not just a `log`/`fmt.Print`.

**How to apply:** For any upload/transfer change, make failures flow through the
`Reporter` and the returned error, and keep the GUI subscriptions in sync (not
just the CLI). ZPA validates laxly (it accepted a room literally named "No Room"
with 201), so rely on local validation, not ZPA's response. `zpa.post()` now
returns an error on non-2xx, carrying ZPA's response body. See
[[gui-and-cli-sync]] and [[cli-to-gui-migration]].
