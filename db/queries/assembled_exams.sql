-- The assembled-exam cache. Pulled forward from phase 3e because the plan reads
-- it: ExamsAt, AncodesInPlan and ExamerInPlan are all built on top of it.
--
-- jsonb, deliberately: the document is the denormalised join of tables that are
-- already relational here. Normalising the copy would create a second source of
-- truth for data whose first one is one query away.

-- name: ListAssembledExams :many
select exam, format_version from assembled_exam
where semester_id = $1
order by ancode;

-- The examer lives inside the document. Mongo indexed nothing here either and
-- scanned the collection; at ~320 exams both are a scan of the same size.
-- The key is `zpaExam`/`mainExamerID` -- the json tags, camelCase -- and no
-- longer the lowercased Go field names bson produced.
--
-- @examer_id and not $2: with a cast on the left-hand side sqlc infers the
-- parameter's type from the expression and would call it `Exam []byte`.
-- name: ListAssembledExamsForExamer :many
select exam, format_version from assembled_exam
where semester_id = @semester_id
  and (exam -> 'zpaExam' ->> 'mainExamerID')::int = @examer_id::int
order by ancode;

-- name: GetAssembledExam :one
select exam, format_version from assembled_exam
where semester_id = $1 and ancode = $2;

-- name: CountAssembledExams :one
select count(*) from assembled_exam where semester_id = $1;

-- name: UpsertAssembledExam :exec
insert into assembled_exam (semester_id, ancode, exam, format_version)
values ($1, $2, $3, $4)
on conflict (semester_id, ancode) do update set
    exam           = excluded.exam,
    format_version = excluded.format_version;

-- name: DeleteAssembledExams :execrows
delete from assembled_exam where semester_id = $1;

-- name: DeleteAssembledExamsState :exec
delete from assembled_exams_state where semester_id = $1;
