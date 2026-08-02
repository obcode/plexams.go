-- name: GetNta :one
select * from nta where mtknr = $1;

-- name: ListNtas :many
select * from nta order by name;

-- name: InsertNta :one
insert into nta (
    mtknr, name, email, compensation, delta_duration_percent,
    needs_room_alone, needs_hardware, program, valid_from, valid_until,
    last_semester, deactivated
) values (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)
returning *;

-- ReplaceNta mirrors the Mongo ReplaceOne: it does NOT upsert. A missing row
-- changes nothing and returns no row, which the caller maps to (nil, nil).
-- name: ReplaceNta :one
update nta set
    name                   = $2,
    email                  = $3,
    compensation           = $4,
    delta_duration_percent = $5,
    needs_room_alone       = $6,
    needs_hardware         = $7,
    program                = $8,
    valid_from             = $9,
    valid_until            = $10,
    last_semester          = $11,
    deactivated            = $12
where mtknr = $1
returning *;

-- name: SetNtaDeactivated :one
update nta set deactivated = $2 where mtknr = $1 returning *;
