-- Program discovery. Under MongoDB this was a regex over collection names
-- (`^exams_[A-Z]{2,4}(-[BM])?$`), which is also why DropPrimussData had to drop
-- the collections rather than empty them: an empty exams_XY kept the program
-- visible forever. Here "has exams" is the definition, so deleting the rows makes
-- the program disappear by itself.
-- The DISTINCT happens in a subquery: SELECT DISTINCT refuses to order by an
-- expression that is not in its select list, and putting the collate there makes
-- sqlc lose the column type.
-- name: ListPrimussPrograms :many
select p.program from (
    select distinct program from primuss_exam where semester_id = $1
) p
order by p.program collate "C";

-- name: GetPrimussExam :one
select * from primuss_exam where semester_id = $1 and program = $2 and ancode = $3;

-- name: PrimussExamExists :one
select exists (
    select 1 from primuss_exam where semester_id = $1 and program = $2 and ancode = $3
);

-- name: ListPrimussExamsForProgram :many
select * from primuss_exam
where semester_id = $1 and program = $2
order by ancode;

-- One ancode across all programs. The Mongo version looped over the programs and
-- did one lookup each, logging (and swallowing) a not-found per program.
-- name: ListPrimussExamsForAncode :many
select * from primuss_exam
where semester_id = $1 and ancode = $2
order by program collate "C";

-- Renumbering a Primuss exam. The counter and both sides of every conflict follow
-- by ON UPDATE CASCADE, so this single statement replaces what used to be three
-- writes plus a $rename over field names.
-- name: ChangePrimussExamAncode :one
update primuss_exam set ancode = $4
where semester_id = $1 and program = $2 and ancode = $3
returning *;

-- name: DeletePrimussExamsForSemester :exec
delete from primuss_exam where semester_id = $1;

-- studentreg has no foreign key to primuss_exam (source data, nothing hand-entered
-- hangs off it), so it is not reached by the cascade and has to be cleared here.
-- name: DeleteStudentRegsForSemester :exec
delete from studentreg where semester_id = $1;
