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

- `plexams/messages.go`: `wrapGerman(err, format, …)` puts a German sentence in front of an
  error and keeps its code (busy, protected semester survive); `p.newExamNamer(ctx, entries)`
  names exams as "Modul (Prüfer:in, Ancode 123)" with `.at()` adding " am Mo 25.01. 10:30";
  it falls back to ZPA and external exams. Use it in every new message.
- NOT_FOUND, ALREADY_EXISTS and INVALID_INPUT carry a precise German sentence; the GUI shows it
  as is (generic text only as fallback). So these messages must be German and actionable.
- `EXTERNAL_SMTP` is set by `email/dial.go`; the server's answer stays in the error chain
  (`smtpFailure`), the planner reads the German text.
- Since 2026-11-26 every validator meets `validation_messages_test.go`; `validatorsToRewrite`
  is empty, so a new finding without German text or (for error/warning) a hint fails the test.

**Why:** the GUI used to match English message text with regular expressions in seven
places, so no message could be reworded without silently breaking a button or a notice.

**How to apply:** an error the GUI must tell apart gets a code, and tests assert on the
code. Message wording is then free to change; user-visible wording is German.
