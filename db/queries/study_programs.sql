-- Shortnames are codes (IF-B, DC-M, MUC.DAI), not prose: order by bytes so the
-- list does not depend on the cluster's collation. Same reasoning as room names.
-- name: ListStudyPrograms :many
select * from study_program order by shortname collate "C";

-- name: GetStudyProgram :one
select * from study_program where shortname = $1;

-- name: UpsertStudyProgram :exec
insert into study_program (
    shortname, name, degree, zpa_code, category, active, retired,
    external_exams_base, joint_faculty
) values (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
)
on conflict (shortname) do update set
    name                = excluded.name,
    degree              = excluded.degree,
    zpa_code            = excluded.zpa_code,
    category            = excluded.category,
    active              = excluded.active,
    retired             = excluded.retired,
    external_exams_base = excluded.external_exams_base,
    joint_faculty       = excluded.joint_faculty;

-- name: DeleteStudyProgram :execrows
delete from study_program where shortname = $1;
