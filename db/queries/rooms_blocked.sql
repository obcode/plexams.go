-- Rooms unavailable at a given time. Unlike the room plan, the start time here is
-- the block's own -- not a copy of an exam's -- so it stays a column.

-- name: ListBlockedRooms :many
select * from blocked_room
where semester_id = $1
order by room_name collate "C", starttime;

-- name: UpsertBlockedRoom :exec
insert into blocked_room (semester_id, room_name, starttime, reason)
values ($1, $2, $3, $4)
on conflict (semester_id, room_name, starttime) do update set
    reason = excluded.reason;

-- name: DeleteBlockedRoom :execrows
delete from blocked_room
where semester_id = $1 and room_name = $2 and starttime = $3;
