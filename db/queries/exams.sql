-- Exams, ZPA-imported and externally owned, plus the "do we plan it" decision.
--
-- Reads that drive planning filter on `withdrawn_at is null`. Under MongoDB an
-- exam ZPA stopped delivering simply vanished on the next import, taking the
-- planner's constraints with it as orphans; here it is marked and everything
-- hanging off it stays.

-- name: ListExamsBySource :many
select * from exam
where semester_id = $1 and source = $2 and withdrawn_at is null
order by ancode;

-- name: GetExam :one
select * from exam
where semester_id = $1 and ancode = $2 and withdrawn_at is null;

-- The ZPA import UPSERTS. It must not drop and re-insert as CacheZPAExams did:
-- the overlay tables reference these rows, and wiping the table would cascade the
-- planner's decisions away with it. Reappearing clears withdrawn_at.
-- name: UpsertZPAExam :exec
insert into exam (
    semester_id, ancode, source, zpa_id, module, main_examer, main_examer_id,
    exam_type, exam_type_full, zpa_date, zpa_starttime, duration_min,
    is_repeater_exam, groups, faculty, withdrawn_at
) values (
    $1, $2, 'zpa', $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, null
)
on conflict (semester_id, ancode) do update set
    source           = 'zpa',
    zpa_id           = excluded.zpa_id,
    module           = excluded.module,
    main_examer      = excluded.main_examer,
    main_examer_id   = excluded.main_examer_id,
    exam_type        = excluded.exam_type,
    exam_type_full   = excluded.exam_type_full,
    zpa_date         = excluded.zpa_date,
    zpa_starttime    = excluded.zpa_starttime,
    duration_min     = excluded.duration_min,
    is_repeater_exam = excluded.is_repeater_exam,
    groups           = excluded.groups,
    faculty          = excluded.faculty,
    withdrawn_at     = null;

-- The other half of the import: what ZPA no longer delivers is marked, never
-- deleted. An exam that is already marked keeps its original timestamp, so the
-- report can say how long it has been gone.
-- name: WithdrawZPAExamsExcept :exec
update exam set withdrawn_at = @at::timestamptz
where semester_id = @semester_id and source = 'zpa'
  and withdrawn_at is null
  and ancode <> all(@keep::int[]);

-- name: ListZPAPrimussAncodes :many
select ancode, program, primuss_ancode from exam_primuss_ancode
where semester_id = $1 and source = 'zpa'
order by ancode, program collate "C";

-- name: ListZPAPrimussAncodesForExam :many
select program, primuss_ancode from exam_primuss_ancode
where semester_id = $1 and ancode = $2 and source = 'zpa'
order by program collate "C";

-- name: DeleteZPAPrimussAncodesForExam :exec
delete from exam_primuss_ancode
where semester_id = $1 and ancode = $2 and source = 'zpa';

-- name: InsertZPAPrimussAncode :exec
insert into exam_primuss_ancode (semester_id, ancode, program, primuss_ancode, source)
values ($1, $2, $3, $4, 'zpa')
on conflict (semester_id, ancode, program, primuss_ancode) do nothing;

-- Absence of a row means UNDECIDED, a real third state: the ZPA import
-- auto-preselects only the exams nobody has decided about yet.
-- name: ListExamsToPlan :many
select ancode from exam_to_plan
where semester_id = $1
order by ancode;

-- name: ListExamsToPlanFiltered :many
select ancode from exam_to_plan
where semester_id = $1 and to_plan = $2
order by ancode;

-- name: SetExamToPlan :exec
insert into exam_to_plan (semester_id, ancode, to_plan)
values ($1, $2, $3)
on conflict (semester_id, ancode) do update set to_plan = excluded.to_plan;

-- name: DeleteExamsToPlan :exec
delete from exam_to_plan where semester_id = $1;

-- ---------------------------------------------------------------------------
-- Externally owned exams: joint programs and other faculties. Same table as the
-- ZPA ones, told apart by `source` -- plan entries and constraints reference
-- "an exam that exists", and SQL cannot key into the union of two tables.
-- ---------------------------------------------------------------------------

-- name: ListExternalExams :many
select * from exam
where semester_id = $1 and source = 'external'
order by ancode;

-- name: GetExternalExam :one
select * from exam
where semester_id = $1 and ancode = $2 and source = 'external';

-- name: InsertExternalExam :exec
insert into exam (
    semester_id, ancode, source, zpa_id, module, main_examer, main_examer_id,
    exam_type, exam_type_full, zpa_date, zpa_starttime, duration_min,
    is_repeater_exam, groups, faculty
) values (
    $1, $2, 'external', $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
);

-- Only an external exam: deleting a ZPA one by hand is not what this method is
-- for, and the ZPA import must never delete at all.
-- name: DeleteExternalExam :exec
delete from exam
where semester_id = $1 and ancode = $2 and source = 'external';

-- name: SetExternalExamFaculty :exec
update exam set faculty = $3
where semester_id = $1 and ancode = $2 and source = 'external';

-- name: ListPrimussAncodesForExternalExams :many
select ancode, program, primuss_ancode from exam_primuss_ancode
where semester_id = $1 and source = 'external'
order by ancode, program collate "C";

-- name: ListPrimussAncodesForExternalExam :many
select program, primuss_ancode from exam_primuss_ancode
where semester_id = $1 and ancode = $2 and source = 'external'
order by program collate "C";

-- name: InsertExternalPrimussAncode :exec
insert into exam_primuss_ancode (semester_id, ancode, program, primuss_ancode, source)
values ($1, $2, $3, $4, 'external')
on conflict (semester_id, ancode, program, primuss_ancode) do nothing;

-- name: DeletePlanEntry :exec
delete from plan_entry where semester_id = $1 and ancode = $2;
