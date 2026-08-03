-- Students who did not fit into any room during room generation. One row per
-- exam: the generator groups by ancode (plexams/roomplan_build.go:416), which is
-- why this table -- unlike planned_room -- is keyed by the ancode alone.

-- Starttime is joined from the plan entry, not stored -- the same treatment
-- planned_room got, for the same reason. No view of its own: there is exactly one
-- reader.
-- name: ListUnplacedExams :many
select u.*, pe.starttime
from unplaced_exam u
join plan_entry pe using (semester_id, ancode)
where u.semester_id = $1
order by u.ancode;

-- name: UpsertUnplacedExam :exec
insert into unplaced_exam (semester_id, ancode, mtknrs, nta_mtknr)
values ($1, $2, $3, $4)
on conflict (semester_id, ancode) do update set
    mtknrs    = excluded.mtknrs,
    nta_mtknr = excluded.nta_mtknr;

-- name: DeleteUnplacedExams :exec
delete from unplaced_exam where semester_id = $1;
