-- Writing the Primuss Sammellisten. What used to be a drop-and-insert of
-- untyped documents into five collections per program is a delete-and-insert of
-- typed rows into four tables, inside one transaction per program.
--
-- Order matters and the foreign keys enforce it: primuss_count and
-- primuss_conflict reference primuss_exam, so the catalogue has to be written
-- before the numbers that talk about it.

-- name: DeletePrimussExamsOfProgram :exec
delete from primuss_exam where semester_id = $1 and program = $2;

-- name: InsertPrimussExam :exec
insert into primuss_exam (
    semester_id, program, ancode, module, main_examer, exam_type, presence, raw
) values ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: DeleteStudentRegsOfProgram :exec
delete from studentreg where semester_id = $1 and program = $2;

-- Named ImportStudentReg because InsertStudentReg is the manual single-row
-- add in studentregs.sql: that one knows neither the student's own program nor
-- the AASPF, and must not start pretending it does.
-- name: ImportStudentReg :exec
insert into studentreg (
    semester_id, program, student_program, primuss_ancode, mtknr, group_name,
    name, presence, aaspf, raw
) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: ListStudentRegRowsOfProgram :many
select primuss_ancode, mtknr, student_program, group_name, name, presence, aaspf, raw
from studentreg
where semester_id = $1 and program = $2
order by primuss_ancode, mtknr;

-- name: DeletePrimussCountsOfProgram :exec
delete from primuss_count where semester_id = $1 and program = $2;

-- name: InsertPrimussCount :exec
insert into primuss_count (semester_id, program, ancode, total, raw)
values ($1, $2, $3, $4, $5);

-- name: DeletePrimussConflictsOfProgram :exec
delete from primuss_conflict where semester_id = $1 and program = $2;

-- name: InsertPrimussConflict :exec
insert into primuss_conflict (
    semester_id, program, ancode, other_ancode, num_students
) values ($1, $2, $3, $4, $5);

-- Which ancodes the catalogue of a program knows. The importer needs it to tell
-- a conflict naming an unknown exam from one it can store -- under MongoDB such
-- a cell was written anyway, as a field name nobody could resolve.
-- name: ListPrimussAncodesOfProgram :many
select ancode from primuss_exam
where semester_id = $1 and program = $2
order by ancode;
