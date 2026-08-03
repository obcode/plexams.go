-- The schedule: one row per exam that has been placed, or that is known to the
-- plan but not yet placed (starttime null).
--
-- Nothing here reads or writes a room's start time. That is the whole point of
-- planned_room_v -- see db/queries/rooms.sql.

-- name: ListPlanEntries :many
select * from plan_entry
where semester_id = $1
order by ancode;

-- name: ListPlanEntriesAt :many
select * from plan_entry
where semester_id = $1 and starttime = $2
order by ancode;

-- name: GetPlanEntry :one
select * from plan_entry
where semester_id = $1 and ancode = $2;

-- AddExamToSlot replaced the whole document: Mongo deleted every entry for the
-- ancode and inserted the new one, so the flags came from the passed entry and
-- not from what was there before. An upsert that sets every column is the same
-- write, minus the window in which the exam was unplanned.
-- name: UpsertPlanEntry :exec
insert into plan_entry (semester_id, ancode, starttime, locked, phase_fixed, external)
values ($1, $2, $3, $4, $5, $6)
on conflict (semester_id, ancode) do update set
    starttime   = excluded.starttime,
    locked      = excluded.locked,
    phase_fixed = excluded.phase_fixed,
    external    = excluded.external;

-- name: SetPlanEntryLocked :exec
update plan_entry set locked = $3
where semester_id = $1 and ancode = $2;

-- name: SetPlanEntryPhaseFixed :exec
update plan_entry set phase_fixed = $3
where semester_id = $1 and ancode = $2;

-- name: ClearAllPhaseFixed :exec
update plan_entry set phase_fixed = false
where semester_id = $1 and phase_fixed;

-- name: LockWholePlan :execrows
update plan_entry set locked = true
where semester_id = $1 and not locked;

-- Everything the generator produced, which is everything that is neither an
-- external entry, nor manually locked, nor frozen by phase A. Mongo spelled the
-- three exclusions as `$ne: true`, which also matched documents where the field
-- was absent; the columns are NOT NULL here, so plain negation is the same set.
-- name: DeleteGeneratedPlanEntries :execrows
delete from plan_entry
where semester_id = $1 and not external and not locked and not phase_fixed;
