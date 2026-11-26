---
name: planner-error-codes
description: Planner API errors carry a stable code (extensions.code, LogLine.code, ValidationFinding.code); the GUI reacts to codes, never to wording
metadata:
  type: project
---

Since 2026-11-26 the planning API reports errors with a code, as the portal already did.

- Codes live in `plexams/apierr` (`CodeBusy`, `CodePublishedLocked`, `CodeAlreadySent`,
  `CodeAlreadyExists`, `CodeReadOnlyRole`, `CodeSemesterReadOnly`, `CodeForbidden`,
  `CodeInternal`, ...). Build one with `apierr.New` / `apierr.Newf`.
- `graph/errors.go`: `presentError` puts the code into `extensions.code`; `emitError`
  puts it on the ERROR `LogLine` of a stream; `clientError` turns `pgx.ErrNoRows` and
  `*pgconn.PgError` into `INTERNAL` with a neutral message (details stay in the log).
- `ValidationFinding` has `code` (set where the GUI offers an action, e.g.
  `NTA_ROOM_NOT_ALONE`) and `hint` (what the planner should do), filled from `ref`.
- A protected semester answers REST uploads with 423, a running operation with 409.

**Why:** the GUI used to match English message text with regular expressions in seven
places, so no message could be reworded without silently breaking a button or a notice.

**How to apply:** an error the GUI must tell apart gets a code, and tests assert on the
code. Message wording is then free to change; user-visible wording is German.
