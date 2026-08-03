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
