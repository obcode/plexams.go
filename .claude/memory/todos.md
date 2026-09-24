---
name: todos
description: "Todo list per semester (tables todo/todo_comment/todo_link, graph/todo.graphqls): links by (kind,key) without FK, labels resolved on read, read-only-semester exemption separate from the VIEWER check, carry-over by copy"
metadata:
  type: project
---

Added 2026-09-24, decided with Oliver: planners' own todos, created and worked off in
the GUI, shown on the landing page and next to linked data (GUI side: `gui/todos.md`).

**Model.** `todo` per semester (title, Markdown description, priority LOW/NORMAL/HIGH,
`due_date date`, `labels text[]`, `recurring`, done_at/done_by, created_by = email),
`todo_comment` (Markdown, only the author may edit), `todo_link (kind, key)`. Status is
just open/done. Authors are emails; names come from a join on `app_user` with the email as
fallback.

**Links carry no foreign key to the target**, on purpose: phases/conditions are Go
constants ([[planning-state-model]]), teachers are delete+insert on every ZPA import, and
rooms/study programs/NTAs are global. `plexams.validateTodoLink` checks the target on
write; labels are resolved on read (SQL `case` in `ListTodoLinks` for entities, Go
`decorateTodoLink` for phase/condition/day/Jira), so a vanished target shows its bare key
and stays removable. `ResolveTodoLinkLabel` repeats the SQL `case` -- keep both in sync.
`href` is computed in the backend (GUI paths like `/exam/assembledExams/<ancode>`).

**Read-only semesters.** Todo mutations stay allowed there and while a validation runs
(`todoMutations` / `isTodoOnlyOperation` in `graph/readonly.go`). They are deliberately
NOT in `readOnlyExemptMutations`: that list also exempts from the VIEWER check, and
VIEWERs must stay read-only. `TestTodoMutationsAreListed` fails when a new `*Todo*`
mutation is missing from the list.

**Carry-over** (`carryOverTodos(fromSemester)`): copies open + recurring todos into the
active semester, open, without due date; EXAM and DAY links are dropped (named in the
copied description instead); still-open originals are closed with a comment.
Idempotent through `carried_from_*` + a unique index. Nothing is automatic -- the GUI
offers a button.

**sqlc:** `date` needed an override like `timestamptz` (sqlc.yaml); pgx scans a date as
midnight UTC, so it is only ever formatted with `time.DateOnly`, never compared with
`time.Local` values.
