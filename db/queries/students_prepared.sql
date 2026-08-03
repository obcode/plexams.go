-- The prepared per-student view: every student with their registrations, ZPA data
-- and NTA resolved in one place. A computed cache, rebuilt by PrepareStudentRegs,
-- so the student is one jsonb document rather than a normalised second copy of
-- data that already exists relationally.
--
-- Ordering goes through the document (model.Student's json tag is "name") instead
-- of a duplicated name column, so there is nothing that can drift out of step
-- with the blob.

-- name: CountStudentsPrepared :one
select count(*) from student_prepared where semester_id = $1;

-- name: ListStudentsPrepared :many
select student, format_version from student_prepared
where semester_id = $1
order by student ->> 'name';

-- name: GetStudentPrepared :one
select student, format_version from student_prepared
where semester_id = $1 and mtknr = $2;

-- name: ListStudentsPreparedByMtknr :many
select student, format_version from student_prepared
where semester_id = $1 and mtknr = any(@mtknrs::text[])
order by student ->> 'name';

-- name: DeleteStudentsPrepared :exec
delete from student_prepared where semester_id = $1;

-- name: InsertStudentPrepared :exec
insert into student_prepared (semester_id, mtknr, student, format_version)
values ($1, $2, $3, $4)
on conflict (semester_id, mtknr) do update set
    student        = excluded.student,
    format_version = excluded.format_version;

-- name: GetStudentRegsState :one
select dirty, reason, changed_at from student_regs_state where semester_id = $1;

-- name: SetStudentRegsState :exec
insert into student_regs_state (semester_id, dirty, reason, changed_at)
values ($1, $2, $3, $4)
on conflict (semester_id) do update set
    dirty      = excluded.dirty,
    reason     = excluded.reason,
    changed_at = excluded.changed_at;

-- The ZPA upload rejects, kept so the planner can see what did not go through.
-- name: DeleteStudentRegUploadErrors :exec
delete from studentreg_upload_error where semester_id = $1;

-- name: InsertStudentRegUploadError :exec
insert into studentreg_upload_error (semester_id, registration, error, format_version)
values ($1, $2, $3, $4);

-- name: ListStudentRegUploadErrors :many
select registration, error, format_version from studentreg_upload_error
where semester_id = $1
order by id;

-- The students that have an NTA, for the NTA mails. `nta` is an optional
-- sub-document of model.Student, so "has one" is jsonb key presence -- the same
-- question Mongo asked with {nta: {$ne: null}}.
-- name: ListStudentsWithNta :many
select student, format_version from student_prepared
where semester_id = $1 and student -> 'nta' is not null
order by student -> 'nta' ->> 'name';
