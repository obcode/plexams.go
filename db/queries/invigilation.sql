-- Aufsichten. invigilations_self and invigilations_other are one table told apart
-- by is_self_invigilation -- the distinction is a property of the row, not of a
-- collection. Every read that used to concatenate two cursors is one query here.
--
-- The `reserve` pseudo-room: a duty not tied to a room is stored with a NULL
-- room_name and is_reserve. The Mongo filters spelled that out as
-- {roomname: nil, isreserve: true}; here it stays two predicates for the same
-- reason -- a room duty must not match the reserve slot and the other way round.

-- name: ListInvigilations :many
select * from invigilation
where semester_id = $1
order by starttime, room_name collate "C" nulls last, invigilator_id;

-- name: ListInvigilationsBySelf :many
select * from invigilation
where semester_id = $1 and is_self_invigilation = $2
order by starttime, room_name collate "C" nulls last, invigilator_id;

-- name: ListInvigilationsForInvigilator :many
select * from invigilation
where semester_id = $1 and invigilator_id = $2
order by starttime, room_name collate "C" nulls last;

-- name: ListInvigilationsInRoomAt :many
select * from invigilation
where semester_id = $1 and starttime = $2
  and room_name = $3 and not is_reserve
order by invigilator_id;

-- name: ListReserveInvigilationsAt :many
select * from invigilation
where semester_id = $1 and starttime = $2
  and room_name is null and is_reserve
order by invigilator_id;

-- AddInvigilationAt replaced the document for that room (or the reserve) at that
-- time, so a second call for the same slot moves the duty rather than adding one.
-- The delete + insert is that replace; there is no unique key to hang an upsert
-- on, because the generated invigilations legitimately repeat per room and slot.
-- name: DeleteInvigilationInRoomAt :exec
delete from invigilation
where semester_id = $1 and starttime = $2
  and not is_self_invigilation
  and room_name is not distinct from $3
  and is_reserve = $4;

-- name: InsertGeneratedInvigilation :exec
insert into invigilation (
    semester_id, invigilator_id, starttime, room_name, duration_min,
    is_reserve, is_self_invigilation, pre_planned
) values ($1, $2, $3, $4, $5, $6, false, $7);

-- is_reserve is passed rather than derived from `room_name is null` in SQL: the
-- caller already knows it (the Mongo filter spelled it `isreserve: roomName ==
-- nil`), and deriving it here makes sqlc see the same parameter as both text and
-- *text.
-- name: SetInvigilationPrePlannedAt :execrows
update invigilation set pre_planned = $5
where semester_id = $1 and starttime = $2
  and not is_self_invigilation
  and room_name is not distinct from $3
  and is_reserve = $4;

-- name: DeleteGeneratedInvigilations :exec
delete from invigilation
where semester_id = $1 and not is_self_invigilation;

-- The pre-planning, keyed by (starttime, room_name) -- see the table comment in
-- 00006 for why not by the invigilator.

-- name: ListPrePlannedInvigilations :many
select * from pre_planned_invigilation
where semester_id = $1
order by starttime, room_name collate "C" nulls last;

-- name: ListPrePlannedInvigilationsForInvigilator :many
select * from pre_planned_invigilation
where semester_id = $1 and invigilator_id = $2
order by starttime, room_name collate "C" nulls last;

-- name: UpsertPrePlannedInvigilation :exec
insert into pre_planned_invigilation (
    semester_id, invigilator_id, starttime, room_name, is_reserve
) values ($1, $2, $3, $4, $5)
on conflict (semester_id, starttime, room_name) do update set
    invigilator_id = excluded.invigilator_id,
    is_reserve     = excluded.is_reserve;

-- name: DeletePrePlannedInvigilationAt :execrows
delete from pre_planned_invigilation
where semester_id = $1 and starttime = $2 and room_name is not distinct from $3;

-- The ZPA-sourced requirements, written by ReplaceAll (db/save.go).

-- name: ListInvigilatorRequirements :many
select * from invigilator_requirement
where semester_id = $1
order by invigilator_id;

-- name: GetInvigilatorRequirement :one
select * from invigilator_requirement
where semester_id = $1 and invigilator_id = $2;

-- The computed fair-share summary. One row per semester, which is what the
-- fixed-_id document always meant -- the primary key is now what the comment in
-- db/invigilator.go had to arrange by hand against interleaved writers.

-- name: GetInvigilationTodos :one
select todos, format_version from invigilation_todos where semester_id = $1;

-- name: UpsertInvigilationTodos :exec
insert into invigilation_todos (semester_id, todos, format_version)
values ($1, $2, $3)
on conflict (semester_id) do update set
    todos          = excluded.todos,
    format_version = excluded.format_version;

-- The planner's per-invigilator constraints, edited in the GUI.

-- name: ListInvigilatorConstraints :many
select * from invigilator_constraint
where semester_id = $1
order by teacher_id;

-- name: GetInvigilatorConstraint :one
select * from invigilator_constraint
where semester_id = $1 and teacher_id = $2;

-- name: UpsertInvigilatorConstraint :exec
insert into invigilator_constraint (semester_id, teacher_id, is_not_invigilator)
values ($1, $2, $3)
on conflict (semester_id, teacher_id) do update set
    is_not_invigilator = excluded.is_not_invigilator;

-- name: DeleteInvigilatorConstraint :execrows
delete from invigilator_constraint where semester_id = $1 and teacher_id = $2;

-- name: ListInvigilatorExcludedDates :many
select teacher_id, excluded_on from invigilator_excluded_date
where semester_id = $1
order by teacher_id, excluded_on;

-- name: ListInvigilatorExcludedDatesForTeacher :many
select excluded_on from invigilator_excluded_date
where semester_id = $1 and teacher_id = $2
order by excluded_on;

-- name: InsertInvigilatorExcludedDate :exec
insert into invigilator_excluded_date (semester_id, teacher_id, excluded_on)
values ($1, $2, $3)
on conflict do nothing;

-- name: DeleteInvigilatorExcludedDates :exec
delete from invigilator_excluded_date where semester_id = $1 and teacher_id = $2;

-- name: ListInvigilatorTimeWindows :many
select teacher_id, window_date, available_from, available_until
from invigilator_time_window
where semester_id = $1
order by teacher_id, window_date;

-- name: ListInvigilatorTimeWindowsForTeacher :many
select window_date, available_from, available_until
from invigilator_time_window
where semester_id = $1 and teacher_id = $2
order by window_date;

-- name: InsertInvigilatorTimeWindow :exec
insert into invigilator_time_window (
    semester_id, teacher_id, window_date, available_from, available_until
) values ($1, $2, $3, $4, $5)
on conflict (semester_id, teacher_id, window_date) do update set
    available_from  = excluded.available_from,
    available_until = excluded.available_until;

-- name: DeleteInvigilatorTimeWindows :exec
delete from invigilator_time_window where semester_id = $1 and teacher_id = $2;
