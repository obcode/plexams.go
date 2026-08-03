-- name: GetRoom :one
select * from room where name = $1;

-- name: RoomExists :one
select exists (select 1 from room where name = $1);

-- Room names are identifiers (R1.006, R3.013, T3.017), not prose. Ordering them
-- by bytes keeps the list independent of the cluster's collation: under a de_DE
-- locale the punctuation is weighted differently and R1.006/R1.06 swap places.
-- name: ListRooms :many
select * from room order by name collate "C";

-- needs_request is a generated column (request_with <> 'NONE') and must not be
-- written. That is the whole point: in Mongo it was stored, under two different
-- keys, and could drift from request_with.
-- name: InsertRoom :one
insert into room (
    name, seats, handicap, lab, places_with_socket, request_with,
    request_priority, exahm, seb, seb_seats, hmeb_seats, deactivated, hitzewert
) values (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
)
returning *;

-- ReplaceRoom mirrors the Mongo ReplaceOne: a full overwrite of the row, no
-- upsert. A missing room changes nothing and returns no row.
-- name: ReplaceRoom :one
update room set
    seats              = $2,
    handicap           = $3,
    lab                = $4,
    places_with_socket = $5,
    request_with       = $6,
    request_priority   = $7,
    exahm              = $8,
    seb                = $9,
    seb_seats          = $10,
    hmeb_seats         = $11,
    deactivated        = $12,
    hitzewert          = $13
where name = $1
returning *;

-- name: SetRoomDeactivated :one
update room set deactivated = $2 where name = $1 returning *;

-- ---------------------------------------------------------------------------
-- The room plan.
--
-- Every read goes through planned_room_v, which joins the exam's plan entry for
-- the start time. The column that used to hold a copy of it is gone, so there is
-- no longer a version of this data that can disagree with the schedule.
--
-- The students are their own table; the reads fold them back into the array the
-- model has always carried, ordered so the answer is stable.
-- ---------------------------------------------------------------------------

-- name: ListPlannedRooms :many
select v.*, coalesce(s.mtknrs, '{}')::text[] as mtknrs
from planned_room_v v
left join lateral (
    select array_agg(prs.mtknr order by prs.mtknr) as mtknrs
    from planned_room_student prs where prs.planned_room_id = v.id
) s on true
where v.semester_id = $1
order by v.ancode, v.room_name collate "C", v.id;

-- name: ListPlannedRoomsAt :many
select v.*, coalesce(s.mtknrs, '{}')::text[] as mtknrs
from planned_room_v v
left join lateral (
    select array_agg(prs.mtknr order by prs.mtknr) as mtknrs
    from planned_room_student prs where prs.planned_room_id = v.id
) s on true
where v.semester_id = $1 and v.starttime = $2
order by v.ancode, v.room_name collate "C", v.id;

-- name: ListPlannedRoomsForAncode :many
select v.*, coalesce(s.mtknrs, '{}')::text[] as mtknrs
from planned_room_v v
left join lateral (
    select array_agg(prs.mtknr order by prs.mtknr) as mtknrs
    from planned_room_student prs where prs.planned_room_id = v.id
) s on true
where v.semester_id = $1 and v.ancode = $2
order by v.room_name collate "C", v.id;

-- Room names, not rooms: the distinct has to happen before the ordering,
-- because `select distinct ... order by x collate "C"` is error 42P10.
-- name: ListPlannedRoomNames :many
select room_name from (
    select distinct room_name from planned_room where semester_id = $1
) r order by room_name collate "C";

-- name: ListPlannedRoomNamesAt :many
select room_name from (
    select distinct room_name from planned_room_v
    where semester_id = $1 and starttime = $2
) r order by room_name collate "C";

-- name: InsertPlannedRoom :one
insert into planned_room (
    semester_id, ancode, room_name, duration_min, handicap,
    handicap_room_alone, reserve, nta_mtknr, pre_planned
) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
returning id;

-- name: InsertPlannedRoomStudent :exec
insert into planned_room_student (planned_room_id, mtknr)
values ($1, $2)
on conflict do nothing;

-- The students go with the rooms: planned_room_student cascades on planned_room.
-- name: DeletePlannedRooms :exec
delete from planned_room where semester_id = $1;

-- ---------------------------------------------------------------------------
-- The pre-planning: rooms the planner pinned by hand, before generation.
-- ---------------------------------------------------------------------------

-- name: ListPrePlannedRooms :many
select * from pre_planned_room
where semester_id = $1
order by ancode, room_name collate "C", mtknr;

-- name: ListPrePlannedRoomsForExam :many
select * from pre_planned_room
where semester_id = $1 and ancode = $2
order by room_name collate "C", mtknr;

-- Mongo deleted the document with the same (ancode, room, mtknr) and inserted the
-- new one. The unique constraint spells that key out, so this is one upsert --
-- and `is not distinct from` is how a NULL mtknr matches the row that has none.
-- name: UpsertPrePlannedRoom :exec
insert into pre_planned_room (semester_id, ancode, room_name, mtknr, reserve, seats)
values ($1, $2, $3, $4, $5, $6)
on conflict (semester_id, ancode, room_name, mtknr) do update set
    reserve = excluded.reserve,
    seats   = excluded.seats;

-- name: DeletePrePlannedRoom :execrows
delete from pre_planned_room
where semester_id = $1 and ancode = $2 and room_name = $3
  and mtknr is not distinct from $4;
