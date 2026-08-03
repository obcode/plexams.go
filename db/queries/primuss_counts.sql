-- Primuss' own registration count per exam. Deliberately a stored number and not
-- a COUNT(*) view: the two drifting apart is a real signal that the validation
-- report exists to catch.
-- name: GetPrimussCount :one
select total from primuss_count where semester_id = $1 and program = $2 and ancode = $3;

-- Normally a no-op: renumbering the exam already moved the counter along by
-- ON UPDATE CASCADE. It stays so the method works when called on its own.
-- name: ChangePrimussCountAncode :exec
update primuss_count set ancode = $4
where semester_id = $1 and program = $2 and ancode = $3;

-- name: IncPrimussCount :exec
update primuss_count set total = total + @delta::int
where semester_id = @semester_id and program = @program and ancode = @ancode;

-- The drift report, as one query instead of two full scans and a map join.
--
-- Only exams that HAVE registrations are considered, exactly as before: a counter
-- row with no registrations at all is not reported. The left join is what makes
-- "there is no counter row" (total is null -> NoCountDocument) distinguishable
-- from "the counter says something else".
-- name: ListStudentRegsCountMismatches :many
select s.primuss_ancode as ancode, count(*)::int as stored, c.total
from studentreg s
left join primuss_count c
       on c.semester_id = s.semester_id
      and c.program     = s.program
      and c.ancode      = s.primuss_ancode
where s.semester_id = $1 and s.program = $2
group by s.primuss_ancode, c.total
having c.total is null or c.total <> count(*)
order by s.primuss_ancode;
