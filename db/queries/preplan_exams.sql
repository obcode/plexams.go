-- Vorplanung: the EXaHM/SEB pseudo-exams that size the Anny bookings before the
-- real ZPA exams are known.
--
-- The id stays an explicit per-semester integer rather than an identity column:
-- the two pair tables reference it, and the CSV round-trip re-imports rows by id
-- so those references survive a restore. That is what UpsertPreplanExam is for.

-- name: ListPreplanExams :many
select * from preplan_exam
where semester_id = $1
order by id;

-- name: GetPreplanExam :one
select * from preplan_exam
where semester_id = $1 and id = $2;

-- max(id)+1, starting at 1. Same answer as the Mongo sort-descending-take-one,
-- and coalesce covers the empty table the way ErrNoDocuments did.
--
-- The ::int cast is needed: max() over an int4 is int8 in sqlc's eyes, and the
-- int override in sqlc.yaml only reaches pg_catalog.int4.
-- name: NextPreplanExamID :one
select (coalesce(max(id), 0) + 1)::int from preplan_exam where semester_id = $1;

-- name: UpsertPreplanExam :exec
insert into preplan_exam (
    semester_id, id, exam_kind, examer_id, examer_name, module, programs,
    expected_students, duration_min, planned_starttime, is_fixed, ancode, notes,
    constraints, format_version
) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
on conflict (semester_id, id) do update set
    exam_kind         = excluded.exam_kind,
    examer_id         = excluded.examer_id,
    examer_name       = excluded.examer_name,
    module            = excluded.module,
    programs          = excluded.programs,
    expected_students = excluded.expected_students,
    duration_min      = excluded.duration_min,
    planned_starttime = excluded.planned_starttime,
    is_fixed          = excluded.is_fixed,
    ancode            = excluded.ancode,
    notes             = excluded.notes,
    constraints       = excluded.constraints,
    format_version    = excluded.format_version;

-- name: DeletePreplanExam :execrows
delete from preplan_exam where semester_id = $1 and id = $2;

-- name: PreplanExamExists :one
select exists (select 1 from preplan_exam where semester_id = $1 and id = $2);

-- The two pair relations. Like exam_same_slot these are canonical unordered
-- pairs, so a read for one pre-exam looks in both columns and a write orders the
-- ids first.

-- name: ListPreplanNotSameSlot :many
select id, other_id from preplan_not_same_slot
where semester_id = $1
order by id, other_id;

-- name: ListPreplanCanShareSlot :many
select id, other_id from preplan_can_share_slot
where semester_id = $1
order by id, other_id;

-- name: InsertPreplanNotSameSlot :exec
insert into preplan_not_same_slot (semester_id, id, other_id)
values ($1, least($2::int, $3::int), greatest($2::int, $3::int))
on conflict do nothing;

-- name: InsertPreplanCanShareSlot :exec
insert into preplan_can_share_slot (semester_id, id, other_id)
values ($1, least($2::int, $3::int), greatest($2::int, $3::int))
on conflict do nothing;

-- name: DeletePreplanNotSameSlotFor :exec
delete from preplan_not_same_slot
where semester_id = @semester_id and (id = @id::int or other_id = @id::int);

-- name: DeletePreplanCanShareSlotFor :exec
delete from preplan_can_share_slot
where semester_id = @semester_id and (id = @id::int or other_id = @id::int);
