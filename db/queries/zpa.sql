-- Teachers and students, both mirrored read-only from ZPA.

-- name: ListTeachers :many
select * from teacher where semester_id = $1 order by fullname;

-- name: GetTeacher :one
select * from teacher where semester_id = $1 and id = $2;

-- Case-insensitive: ZPA stores raw addresses, our user emails are lower-cased.
-- The index is on lower(email) for exactly this query.
-- name: GetTeacherByEmail :one
select * from teacher where semester_id = $1 and lower(email) = lower(@email::text);

-- A substring match on the full name, which is what the Mongo $regex without
-- anchors did. POSIX rather than PCRE, same language for names.
-- name: GetTeacherIDByName :one
select id from teacher
where semester_id = $1 and fullname ~ @name::text
order by id
limit 1;

-- name: DeleteTeachers :exec
delete from teacher where semester_id = $1;

-- name: InsertTeacher :exec
insert into teacher (
    semester_id, id, shortname, fullname, email, is_prof, is_lba, is_prof_hc,
    is_staff, is_active, last_semester, fk
) values (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)
on conflict (semester_id, id) do update set
    shortname     = excluded.shortname,
    fullname      = excluded.fullname,
    email         = excluded.email,
    is_prof       = excluded.is_prof,
    is_lba        = excluded.is_lba,
    is_prof_hc    = excluded.is_prof_hc,
    is_staff      = excluded.is_staff,
    is_active     = excluded.is_active,
    last_semester = excluded.last_semester,
    fk            = excluded.fk;

-- name: ListZPAStudents :many
select * from zpa_student where semester_id = $1 order by last_name, first_name;

-- name: GetZPAStudent :one
select * from zpa_student where semester_id = $1 and mtknr = $2;

-- ---------------------------------------------------------------------------
-- The four ReplaceAll targets (db/save.go). Each is source data replaced
-- wholesale by an import or a generation run, and nothing references any of
-- them -- which is why replacing is safe here and would not be for the exams.
-- ---------------------------------------------------------------------------

-- name: DeleteZPAStudents :exec
delete from zpa_student where semester_id = $1;

-- name: InsertZPAStudent :exec
insert into zpa_student (
    semester_id, mtknr, greeting, first_name, last_name, email, gender, group_name
) values ($1, $2, $3, $4, $5, $6, $7, $8)
on conflict (semester_id, mtknr) do update set
    greeting   = excluded.greeting,
    first_name = excluded.first_name,
    last_name  = excluded.last_name,
    email      = excluded.email,
    gender     = excluded.gender,
    group_name = excluded.group_name;

-- name: DeleteInvigilatorRequirements :exec
delete from invigilator_requirement where semester_id = $1;

-- name: InsertInvigilatorRequirement :exec
insert into invigilator_requirement (
    semester_id, invigilator_id, invigilator, excluded_dates, part_time,
    oral_exams_contribution, livecoding_contribution, master_contribution,
    free_semester, overtime_last_semester, overtime_this_semester
) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
on conflict (semester_id, invigilator_id) do update set
    invigilator             = excluded.invigilator,
    excluded_dates          = excluded.excluded_dates,
    part_time               = excluded.part_time,
    oral_exams_contribution = excluded.oral_exams_contribution,
    livecoding_contribution = excluded.livecoding_contribution,
    master_contribution     = excluded.master_contribution,
    free_semester           = excluded.free_semester,
    overtime_last_semester  = excluded.overtime_last_semester,
    overtime_this_semester  = excluded.overtime_this_semester;

-- The two invigilation targets share one table and are told apart by
-- is_self_invigilation, so each replaces only its own half.
-- name: DeleteInvigilations :exec
delete from invigilation
where semester_id = $1 and is_self_invigilation = $2;

-- name: InsertInvigilation :exec
insert into invigilation (
    semester_id, invigilator_id, starttime, room_name, duration_min,
    is_reserve, is_self_invigilation, pre_planned
) values ($1, $2, $3, $4, $5, $6, $7, $8);
