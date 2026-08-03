-- Room requests to Gebäudemanagement, keyed by (room, starttime) -- see the
-- table comment in 00005 for why not by valid_from.

-- name: ListRoomRequests :many
select * from room_request
where semester_id = $1
order by room_name collate "C", starttime;

-- name: GetRoomRequest :one
select * from room_request
where semester_id = $1 and room_name = $2 and starttime = $3;

-- name: InsertRoomRequest :exec
insert into room_request (
    semester_id, room_name, starttime, valid_from, valid_until, approved, active
) values ($1, $2, $3, $4, $5, $6, $7);

-- The Mongo versions were $set of a single field and returned nil when nothing
-- matched. `returning *` says both at once: no row back means no such request.
-- name: SetRoomRequestApproved :one
update room_request set approved = $4
where semester_id = $1 and room_name = $2 and starttime = $3
returning *;

-- name: SetRoomRequestActive :one
update room_request set active = $4
where semester_id = $1 and room_name = $2 and starttime = $3
returning *;

-- name: SetRoomRequestWindow :one
update room_request set valid_from = $4, valid_until = $5
where semester_id = $1 and room_name = $2 and starttime = $3
returning *;

-- name: DeleteRoomRequests :exec
delete from room_request where semester_id = $1;
