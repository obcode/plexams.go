-- The conflicts of one exam. The LEFT JOIN is what makes "this exam has no
-- conflicts" a row with nulls instead of no row at all: under MongoDB every exam
-- had a conflicts document even when it carried no counterpart keys, and callers
-- rely on getting the module and examer back. Verified across four programs of
-- 2026-SS: exactly one conflicts document per exam, in both directions.
--
-- Module and main examer come from primuss_exam rather than from a second copy in
-- the conflicts document (Titel/Prüfer), which could disagree with the catalogue.
-- name: ListPrimussConflictsForAncode :many
select e.ancode, e.module, e.main_examer, c.other_ancode, c.num_students
from primuss_exam e
left join primuss_conflict c
       on c.semester_id = e.semester_id
      and c.program     = e.program
      and c.ancode      = e.ancode
where e.semester_id = $1 and e.program = $2 and e.ancode = $3
order by c.other_ancode;

-- All conflicts of a program at once, for assembling exams without a per-exam
-- lookup. Same shape, one query instead of a hand-decoded cursor.
-- name: ListPrimussConflictsForProgram :many
select e.ancode, e.module, e.main_examer, c.other_ancode, c.num_students
from primuss_exam e
left join primuss_conflict c
       on c.semester_id = e.semester_id
      and c.program     = e.program
      and c.ancode      = e.ancode
where e.semester_id = $1 and e.program = $2
order by e.ancode, c.other_ancode;
