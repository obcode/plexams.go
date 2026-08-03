-- name: GetSchedulerState :one
select * from scheduler_state where id = 1;

-- TouchSchedulerFire records the start of a fire: the catch-up anchor, the
-- trigger and the workspace, before the run executes. It must not touch the
-- previous run's outcome, so the ON CONFLICT branch lists only these three
-- columns -- the same reason the Mongo version used $set rather than a replace.
-- name: TouchSchedulerFire :exec
insert into scheduler_state (id, last_fire_at, last_trigger, semester_id)
values (1, $1, $2, $3)
on conflict (id) do update set
    last_fire_at = excluded.last_fire_at,
    last_trigger = excluded.last_trigger,
    semester_id  = excluded.semester_id;

-- name: SaveSchedulerState :exec
insert into scheduler_state (
    id, last_fire_at, last_finished, last_status, last_trigger, semester_id, total_changes
) values (
    1, $1, $2, $3, $4, $5, $6
)
on conflict (id) do update set
    last_fire_at  = excluded.last_fire_at,
    last_finished = excluded.last_finished,
    last_status   = excluded.last_status,
    last_trigger  = excluded.last_trigger,
    semester_id   = excluded.semester_id,
    total_changes = excluded.total_changes;
