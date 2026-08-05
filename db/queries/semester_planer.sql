-- A semester's planner override. At most one row per semester; a missing row
-- means the semester inherits the planner from the server config. Every column
-- is nullable for the same reason: nil is "inherit", not "empty".
--
-- The successor of the global `planer` singleton (dropped in migration 00008).

-- name: GetSemesterPlaner :one
select * from semester_planer where semester_id = $1;

-- name: SaveSemesterPlaner :exec
insert into semester_planer (semester_id, name, email, test_mail, cc, noreply_mail, noreply_name)
values ($1, $2, $3, $4, $5, $6, $7)
on conflict (semester_id) do update set
    name         = excluded.name,
    email        = excluded.email,
    test_mail    = excluded.test_mail,
    cc           = excluded.cc,
    noreply_mail = excluded.noreply_mail,
    noreply_name = excluded.noreply_name;

-- name: DeleteSemesterPlaner :exec
delete from semester_planer where semester_id = $1;
