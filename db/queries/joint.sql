-- Exams of joint study programs (MUC.DAI, MUC.HEALTH), imported from their CSV.
-- Was joint_<PROG>, keyed by the German column names of the source file (Nr,
-- Modulname, Prüfungsform, ...); those are a header mapping in the importer now.

-- name: DeleteJointExamsForProgram :exec
delete from joint_exam where semester_id = $1 and program = $2;

-- name: InsertJointExam :exec
insert into joint_exam (
    semester_id, program, primuss_ancode, module, exam_type, grading,
    duration_min, main_examer, second_examer, is_repeater_exam, planer
) values (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
);

-- name: ListJointExamsForProgram :many
select * from joint_exam
where semester_id = $1 and program = $2
order by primuss_ancode;

-- name: GetJointExam :one
select * from joint_exam
where semester_id = $1 and program = $2 and primuss_ancode = $3;

-- The link from a joint exam to the local (external or ZPA) ancode representing
-- it. Stored explicitly so a later ZPA re-import cannot silently break it.
-- name: ListJointLinks :many
select * from joint_link
where semester_id = $1
order by program collate "C", primuss_ancode;

-- name: GetJointLink :one
select * from joint_link
where semester_id = $1 and program = $2 and primuss_ancode = $3;

-- name: UpsertJointLink :exec
insert into joint_link (
    semester_id, program, primuss_ancode, kind, ancode, status, source, module, main_examer
) values (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
)
on conflict (semester_id, program, primuss_ancode) do update set
    kind        = excluded.kind,
    ancode      = excluded.ancode,
    status      = excluded.status,
    source      = excluded.source,
    module      = excluded.module,
    main_examer = excluded.main_examer;

-- name: DeleteJointLink :exec
delete from joint_link
where semester_id = $1 and program = $2 and primuss_ancode = $3;
