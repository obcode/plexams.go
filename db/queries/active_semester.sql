-- The stored answer is only the workspace id; the logical semester comes from the
-- registry rather than being duplicated here. Under MongoDB both were written
-- into the same document and could disagree after a workspace was renamed.
-- name: GetActiveSemester :one
select a.semester_id, s.semester
from active_semester a
join semester s on s.id = a.semester_id
where a.id = 1;

-- name: SaveActiveSemester :exec
insert into active_semester (id, semester_id)
values (1, $1)
on conflict (id) do update set semester_id = excluded.semester_id;
