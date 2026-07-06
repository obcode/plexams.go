---
name: fresh-db-bootstrap
description: Bootstrapping a fresh/empty Mongo — auto-resolve needs an existing workspace; pin a semester to break the chicken-and-egg.
metadata:
  node_type: memory
  type: project
  originSessionId: f9475e51-a612-4bb9-abdf-86531ba34ed6
---

Starting against a fresh/empty Mongo (only the `plexams` bookkeeping DB present) fails: `initPlexamsConfig` auto-selects a start semester via `ResolveStartSemester`, which only returns a workspace that already exists (has a semester_config + compatible schemaVersion). Nothing exists yet → `FATAL`.

**Why:** chicken-and-egg — no workspace means no auto-start, but auto-start is what would open the workspace. `NewPlexams` itself does not fatal on a missing config (only on "cannot connect"); `semesterConfig` just stays nil.

**How to apply:** break it by *pinning* a semester so the code skips auto-resolve (`semesterPinned` branch in cmd/root.go):
- Server (docker stack, no subcommand): put `semester: 2026-SS` in `.plexams.yaml`.
- `plexams.go init 2026-SS` — now auto-pins from `args[0]` (init.go assigns the package-level `semester`), no separate `--semester` needed.
- `plexams.go --semester 2026-SS import semester-dump <file.zip>` — restore a dump into the pinned (fresh) DB.

`resolveStartSemester` now returns `connErr` so the two failure modes have distinct FATAL messages: "cannot connect to the database" (DNS/network, e.g. wrong compose service name for `db.uri` host) vs "database has no usable workspace yet — pin a semester…". Related: [[semester-dump-restore]].
