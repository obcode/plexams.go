---
name: nightly-autosync-zpa-anny
description: "Nightly in-process scheduler that re-pulls ZPA + Anny and mails the diff; backend DONE & on main, GUI-sync pending."
metadata:
  node_type: memory
  type: project
  originSessionId: a4245363-309a-45fb-ae09-45214c2e5879
---

Nightly auto-sync of ZPA + Anny with change-email. Backend DONE, merged to main & pushed 2026-07-15 (commits `4031168` zpaimport, `45738e2` anny, `062362f` plexams, `4507b11` graph, `ff33e3c` docs), builds/vets/lints/tests green.

**What:** in-process goroutine (`plexams/scheduler`, started from `graph.StartServer`, ctx cancelled on shutdown signal) runs daily at `scheduler.time` (local, default 03:00) when `scheduler.enabled`. Calls `Plexams.RunScheduledSync` ([plexams/autosync.go](../../../../workspace/plexams.go/plexams/autosync.go)): `ResetZPA()` (fresh client — the long-lived one caches teachers/exams in memory), re-imports ZPA exams/teachers/invig-reqs + Anny bookings for the **active** workspace via the existing import methods under `TryBeginExclusiveOp` (retries then skips on collision), reads back per-source `SyncLogEntry` diffs, mails: change-mail to `scheduler.changesrecipient` only when something differs + heartbeat to `scheduler.debugrecipient` on every run (also errors/skips). Big first-fill collapses to a count (`maxDetailLines`).

**Key building blocks reused:** `zpaimport.DiffChanges` generalized to `[T any, K comparable]` (int ZPA keys still infer, Anny keys on string `Number`); Anny import now writes a full diff (`plexams/anny/fetch.go` `diffBookings`). The Anny diff is restricted to the **exam period** (`DiffWindow{From,Until}` passed from `semesterConfig` via `FetchFromAnny`; zero window falls back to now) and each entry is marked `[eigene]`/`[fremd]` with the booker (`PersonalizationName` + `MatchesAnyPersonalization`) so mine-vs-others is clear. Manual per-source GUI imports unchanged. On-demand `triggerScheduledSync` subscription streams the same run.

**Decisions:** in-process goroutine (not OS cron); import+diff+mail (not detect-only); active-workspace only in v1 — watching a specific non-active upcoming semester (e.g. 2026-WS while another is active) would need a scoped per-semester instance, deferred.

**Config:** `scheduler.{enabled,time,changesrecipient,debugrecipient,sources.*}` — documented in [deploy/.plexams.yaml.example], [docs/configuration.md] §4a, [CLAUDE.md]. Uses existing zpa/anny/smtp secrets.

**Pending:** GUI-sync — add a "Sync jetzt" button wired to `triggerScheduledSync` (streams LogLine like the other imports) + optionally surface `syncLog`/last-run. Debug-recipient generalization to a real event/error-tracking channel (self-hosted Sentry/GlitchTip/Bugsnag) is a future idea the user floated. Related: [[deploy-push-cd]], [[zpa-import-behaviors]], [[mucdai-import-linking]].
