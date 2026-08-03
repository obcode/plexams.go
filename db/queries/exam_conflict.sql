-- The two hand-entered judgements about a conflict between two exams: "these may
-- share a slot" and, per student, "this one does not count" / "this one counts".
--
-- Both are pairs of ancodes, and both are stored canonically (ancode <
-- other_ancode). plexams normalises with conflictcalc.NormPair before it calls in
-- (plexams/exam_conflict.go:75,93) -- but the CSV import does not
-- (csv_export.go:869,908), so the ordering is done here as well. Under MongoDB a
-- hand-edited CSV row with the ancodes the other way round created a second,
-- unreachable document that the readers then counted twice.

-- name: ListConflictDecisions :many
select ancode, other_ancode, mtknr, decision from exam_conflict_rating
where semester_id = $1
order by ancode, other_ancode, mtknr;

-- name: UpsertConflictDecision :exec
insert into exam_conflict_rating (semester_id, ancode, other_ancode, mtknr, decision)
values ($1, least($2::int, $3::int), greatest($2::int, $3::int), $4, $5)
on conflict (semester_id, ancode, other_ancode, mtknr) do update set
    decision = excluded.decision;

-- name: DeleteConflictDecision :execrows
delete from exam_conflict_rating
where semester_id = $1
  and ancode = least($2::int, $3::int)
  and other_ancode = greatest($2::int, $3::int)
  and mtknr = $4;

-- name: ListCanShareSlotPairs :many
select ancode, other_ancode from exam_can_share_slot
where semester_id = $1
order by ancode, other_ancode;

-- name: UpsertCanShareSlot :exec
insert into exam_can_share_slot (semester_id, ancode, other_ancode)
values ($1, least($2::int, $3::int), greatest($2::int, $3::int))
on conflict do nothing;

-- name: DeleteCanShareSlot :execrows
delete from exam_can_share_slot
where semester_id = $1
  and ancode = least($2::int, $3::int)
  and other_ancode = greatest($2::int, $3::int);
