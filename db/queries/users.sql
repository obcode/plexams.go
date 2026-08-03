-- Email addresses are identifiers: order by bytes, as Mongo did.
-- name: ListUsers :many
select * from app_user order by email collate "C";

-- name: GetUser :one
select * from app_user where email = $1;

-- name: SaveUser :exec
insert into app_user (email, name, role, shortname)
values ($1, $2, $3, $4)
on conflict (email) do update set
    name      = excluded.name,
    role      = excluded.role,
    shortname = excluded.shortname;

-- DeleteUser reports nothing: the Mongo version ignored the deleted count too,
-- and removing an already-absent user is not an error worth surfacing.
-- name: DeleteUser :exec
delete from app_user where email = $1;
