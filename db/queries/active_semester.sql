-- Only the semester is stored. Under MongoDB the document carried the database
-- name AND a logical semester next to it, and the two could disagree.
-- name: GetActiveSemester :one
select semester_id from active_semester where id = 1;

-- name: SaveActiveSemester :exec
insert into active_semester (id, semester_id)
values (1, $1)
on conflict (id) do update set semester_id = excluded.semester_id;
