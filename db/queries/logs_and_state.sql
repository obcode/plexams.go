-- The two audit logs and the two little state tables.
--
-- Both logs keep their payload as jsonb -- the mutation args and the sync diff
-- are read and written whole with their row, never joined and (with one
-- exception below) never queried into.

-- The one exception: the mutation-log filter can ask for an argument key/value
-- pair, which was Mongo's single $elemMatch (db/mutation_log.go:51). The jsonb
-- containment operator says the same thing, and unlike $elemMatch it can use a
-- GIN index later if the log ever grows enough to need one.
--
-- Every filter is optional and spelled the same way: a NULL parameter means "do
-- not filter". A limit of 0 or less means all rows -- the Mongo version simply
-- did not call SetLimit.
--
-- sqlc.narg, not @name: emit_pointers_for_null_types reaches nullable *columns*,
-- not parameters, so a plain @op_type would arrive as a non-nullable string and
-- "no filter" would become "filter on the empty string".
-- name: ListMutationLog :many
select * from mutation_log
where semester_id = @semester_id
  and (sqlc.narg(op_type)::text is null or type = sqlc.narg(op_type))
  and (sqlc.narg(op_name)::text is null or name = sqlc.narg(op_name))
  and (sqlc.narg(user_email)::text is null or user_email = sqlc.narg(user_email))
  and (sqlc.narg(ancode)::int is null or sqlc.narg(ancode)::int = any(ancodes))
  and (sqlc.narg(arg_filters)::jsonb is null or args @> sqlc.narg(arg_filters)::jsonb)
  and (sqlc.narg(since)::timestamptz is null or logged_at >= sqlc.narg(since))
  and (sqlc.narg(until)::timestamptz is null or logged_at <= sqlc.narg(until))
order by logged_at desc, id desc
limit nullif(@max_rows::int, 0);

-- name: InsertMutationLogEntry :exec
insert into mutation_log (
    semester_id, logged_at, name, type, user_email, args, ancodes, error, duration_ms
) values ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: LatestMutationTime :one
select logged_at from mutation_log
where semester_id = $1
order by logged_at desc, id desc
limit 1;

-- The distinct in a subquery, sorted outside: `select distinct ... order by x
-- collate "C"` is error 42P10.
-- name: ListMutationLogNames :many
select name from (
    select distinct name from mutation_log where semester_id = $1
) n order by name collate "C";

-- name: ListSyncLog :many
select * from sync_log
where semester_id = $1
order by logged_at desc, id desc
limit nullif(@max_rows::int, 0);

-- name: InsertSyncLogEntry :exec
insert into sync_log (
    semester_id, logged_at, operation, label, direction, system, ok, summary,
    added, changed, removed, entries, format_version
) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13);

-- The planning state: a set of keys, where presence is truth.

-- name: ListPlanningConditions :many
select condition_key from planning_state
where semester_id = $1
order by condition_key collate "C";

-- name: SetPlanningCondition :exec
insert into planning_state (semester_id, condition_key)
values ($1, $2)
on conflict do nothing;

-- name: UnsetPlanningCondition :exec
delete from planning_state where semester_id = $1 and condition_key = $2;

-- Whether the assembled-exam cache is stale. One row per semester -- the Mongo
-- version replaced "the" document with an empty filter, which is the same thing
-- as long as there is exactly one.

-- name: GetAssembledExamsState :one
select * from assembled_exams_state where semester_id = $1;

-- name: SetAssembledExamsState :exec
insert into assembled_exams_state (semester_id, dirty, reason, changed_at)
values ($1, $2, $3, $4)
on conflict (semester_id) do update set
    dirty      = excluded.dirty,
    reason     = excluded.reason,
    changed_at = excluded.changed_at;

-- Email attachments. The bytes live in the row, as they did in the document.

-- The listing deliberately does NOT select data: it drives a GUI table, and a
-- cover-page PDF per teacher is megabytes nobody asked for. That was Mongo's
-- projection {data: 0}; here it is simply a column list.
-- name: ListEmailAttachmentInfos :many
select kind, key, filename, content_type, size, uploaded_at
from email_attachment
where semester_id = $1 and kind = $2
order by key collate "C";

-- name: GetEmailAttachment :one
select * from email_attachment
where semester_id = $1 and kind = $2 and key = $3;

-- name: UpsertEmailAttachment :exec
insert into email_attachment (
    semester_id, kind, key, filename, content_type, size, data, uploaded_at
) values ($1, $2, $3, $4, $5, $6, $7, $8)
on conflict (semester_id, kind, key) do update set
    filename     = excluded.filename,
    content_type = excluded.content_type,
    size         = excluded.size,
    data         = excluded.data,
    uploaded_at  = excluded.uploaded_at;

-- name: DeleteEmailAttachments :execrows
delete from email_attachment where semester_id = $1 and kind = $2;
