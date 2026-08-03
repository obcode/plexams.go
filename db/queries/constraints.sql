-- Per-exam planning constraints. One model.Constraints is four tables:
-- exam_constraint, exam_same_slot, exam_room_constraint and exam_allowed_room.
--
-- The room constraints are their own table because model.Constraints.
-- RoomConstraints is a POINTER -- the presence of the row is the presence of the
-- constraint, which nullable columns could not express.

-- name: ListExamConstraints :many
select * from exam_constraint
where semester_id = $1
order by ancode;

-- name: GetExamConstraint :one
select * from exam_constraint
where semester_id = $1 and ancode = $2;

-- name: UpsertExamConstraint :exec
insert into exam_constraint (
    semester_id, ancode, not_planned_by_me, not_planned_by_me_fk, do_not_publish,
    online, location, exclude_days, possible_days, fixed_day, fixed_time
) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
on conflict (semester_id, ancode) do update set
    not_planned_by_me    = excluded.not_planned_by_me,
    not_planned_by_me_fk = excluded.not_planned_by_me_fk,
    do_not_publish       = excluded.do_not_publish,
    online               = excluded.online,
    location             = excluded.location,
    exclude_days         = excluded.exclude_days,
    possible_days        = excluded.possible_days,
    fixed_day            = excluded.fixed_day,
    fixed_time           = excluded.fixed_time;

-- name: DeleteExamConstraint :execrows
delete from exam_constraint where semester_id = $1 and ancode = $2;

-- Same-slot pairs are stored once, canonically (ancode < other_ancode), so a
-- read for one exam has to look in both columns. Under MongoDB each exam carried
-- its own list and nothing enforced that the two agreed; here there is one row
-- per pair and the question cannot have two answers.
-- name: ListSameSlotPairs :many
select ancode, other_ancode from exam_same_slot
where semester_id = $1
order by ancode, other_ancode;

-- The cast on the CASE is not decoration: without it sqlc cannot infer the
-- column type and returns []interface{}.
-- name: ListSameSlotForAncode :many
select (case when ancode = @ancode::int then other_ancode else ancode end)::int as other
from exam_same_slot
where semester_id = @semester_id
  and (ancode = @ancode::int or other_ancode = @ancode::int)
order by other;

-- name: InsertSameSlotPair :exec
insert into exam_same_slot (semester_id, ancode, other_ancode)
values ($1, least($2::int, $3::int), greatest($2::int, $3::int))
on conflict do nothing;

-- Everything this exam is paired with, in either direction. Replacing an exam's
-- constraints replaces its whole side of the relation, exactly as replacing the
-- Mongo document did.
-- name: DeleteSameSlotForAncode :exec
delete from exam_same_slot
where semester_id = @semester_id
  and (ancode = @ancode::int or other_ancode = @ancode::int);

-- name: ListExamRoomConstraints :many
select * from exam_room_constraint
where semester_id = $1
order by ancode;

-- name: GetExamRoomConstraint :one
select * from exam_room_constraint
where semester_id = $1 and ancode = $2;

-- name: UpsertExamRoomConstraint :exec
insert into exam_room_constraint (
    semester_id, ancode, places_with_socket, lab, exahm, seb, kdp_jira_url,
    max_students, additional_seats, pre_exam_minutes, post_exam_minutes, comments
) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
on conflict (semester_id, ancode) do update set
    places_with_socket = excluded.places_with_socket,
    lab                = excluded.lab,
    exahm              = excluded.exahm,
    seb                = excluded.seb,
    kdp_jira_url       = excluded.kdp_jira_url,
    max_students       = excluded.max_students,
    additional_seats   = excluded.additional_seats,
    pre_exam_minutes   = excluded.pre_exam_minutes,
    post_exam_minutes  = excluded.post_exam_minutes,
    comments           = excluded.comments;

-- The allowed rooms cascade with it.
-- name: DeleteExamRoomConstraint :exec
delete from exam_room_constraint where semester_id = $1 and ancode = $2;

-- name: ListAllowedRooms :many
select ancode, room_name from exam_allowed_room
where semester_id = $1
order by ancode, room_name collate "C";

-- name: ListAllowedRoomsForAncode :many
select room_name from exam_allowed_room
where semester_id = $1 and ancode = $2
order by room_name collate "C";

-- name: InsertAllowedRoom :exec
insert into exam_allowed_room (semester_id, ancode, room_name)
values ($1, $2, $3)
on conflict do nothing;

-- name: DeleteAllowedRooms :exec
delete from exam_allowed_room where semester_id = $1 and ancode = $2;
