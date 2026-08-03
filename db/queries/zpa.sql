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
