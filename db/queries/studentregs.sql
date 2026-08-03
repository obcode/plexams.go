-- Raw Primuss registrations. `program` is the EXAM's program (the former
-- collection suffix); `student_program` is the student's own (Primuss column
-- Stg) and is what model.StudentReg.Program carries.

-- name: ListStudentRegsForAncode :many
select * from studentreg
where semester_id = $1 and program = $2 and primuss_ancode = $3
order by name;

-- name: ListStudentRegsForProgram :many
select * from studentreg
where semester_id = $1 and program = $2
order by primuss_ancode, name;

-- studentreg has no foreign key to primuss_exam, so this is a plain update and
-- not something the exam rename cascades into. That is deliberate: the
-- registrations are source data and nothing hand-entered hangs off them.
-- name: ChangeStudentRegsAncode :exec
update studentreg set primuss_ancode = $4
where semester_id = $1 and program = $2 and primuss_ancode = $3;

-- Students registered more than once for the same exam. Reported, never
-- enforced: the Primuss source data really does contain such a duplicate, so a
-- unique key would reject the import instead of protecting anything.
-- name: ListDuplicateStudentRegs :many
select primuss_ancode as ancode, mtknr, count(*)::int as n
from studentreg
where semester_id = $1 and program = $2
group by primuss_ancode, mtknr
having count(*) > 1
order by primuss_ancode, mtknr collate "C";

-- Exactly ONE row, because the Mongo DeleteOne deleted exactly one document. It
-- matters precisely in the duplicate case, which is the reason this method
-- exists: deleting every matching row would take the legitimate registration
-- with the duplicate.
-- name: DeleteOneStudentReg :execrows
delete from studentreg where id = (
    select s.id from studentreg s
    where s.semester_id = $1 and s.program = $2 and s.primuss_ancode = $3 and s.mtknr = $4
    order by s.id
    limit 1
);

-- The manual add knows the exam's program and the student's name, but not the
-- student's own program -- the Mongo insert wrote AnCode, MTKNR and name and
-- nothing else, so student_program stays at its default.
-- name: InsertStudentReg :exec
insert into studentreg (semester_id, program, primuss_ancode, mtknr, name)
values ($1, $2, $3, $4, $5);

-- Distinct students whose name matches, across every program. POSIX regex, where
-- MongoDB used its own $regex flavour; for the name searches this serves that is
-- the same language.
-- name: ListStudentMtknrsByName :many
select distinct mtknr from studentreg
where semester_id = $1 and name ~ $2;
