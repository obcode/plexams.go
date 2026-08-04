-- The semester registry: what used to be one MongoDB database per semester is
-- one row here.
--
-- SemesterMeta was a single document in each database's `semester_meta`
-- collection. Its fields are columns of `semester` now, which is why the registry
-- can answer "which semesters exist, and are they compatible?" with one query
-- instead of a ListDatabaseNames plus two probes per database.

-- name: GetSemester :one
select * from semester where id = $1;

-- name: SemesterExists :one
select exists (select 1 from semester where id = $1);

-- Every semester, with whether it carries a config. `compatible` was "has a
-- semester_config_input document"; the left join says the same thing.
--
-- Newest first. That used to sort by the stored label with the id as tie-break,
-- so the canonical database came before a test clone of the same semester; the
-- clones are gone and the id sorts identically ("2026-WS" > "2026-SS" > "2025-WS"
-- both as a label and as an id).
--
-- The ::bool cast is not decoration: without it sqlc types has_config as
-- interface{}.
-- name: ListSemesters :many
select s.*, (c.semester_id is not null)::bool as has_config
from semester s
left join semester_config_input c on c.semester_id = s.id
order by s.id collate "C" desc;

-- Registering a semester and stamping the schema version of a fresh row. An
-- already registered semester keeps its version: this is called on every start,
-- and the version is only ever moved by Migrate.
-- name: EnsureSemester :exec
insert into semester (id, schema_version)
values ($1, $2)
on conflict (id) do update set
    schema_version = case
        when semester.schema_version = 0 then excluded.schema_version
        else semester.schema_version
    end;

-- name: SetSemesterSchemaVersion :exec
update semester set schema_version = $2 where id = $1;

-- name: SetSemesterReadOnly :exec
update semester set read_only = $2 where id = $1;

-- name: SetSemesterLastDumpAt :exec
update semester set last_dump_at = $2 where id = $1;

-- The per-semester planning config. One row per semester, jsonb: it is an
-- editable document read and written whole, and the GUI form is its shape.

-- name: GetSemesterConfigInput :one
select config, format_version from semester_config_input where semester_id = $1;

-- name: SetSemesterConfigInput :exec
insert into semester_config_input (semester_id, config, format_version)
values ($1, $2, $3)
on conflict (semester_id) do update set
    config         = excluded.config,
    format_version = excluded.format_version;

-- The active semester lives in db/queries/active_semester.sql (phase 3a).
