-- The semester registry: what used to be "one MongoDB database per semester",
-- discovered by listing database names.
--
-- semester.id is the workspace key -- the former physical database name, which is
-- NOT always the logical semester: clones like "Test26SS-v2" carry semester
-- "2026 SS" so ZPA still sees the real one. Every semester-scoped table from here
-- on references this id.

-- +goose Up

create table semester (
    -- Workspace key: "2026-WS", "Test26SS-v2". Was the MongoDB database name.
    id             text primary key,
    -- Logical semester as ZPA knows it, e.g. "2026 SS". Deliberately separate
    -- from id so a clone can be planned against without lying to ZPA.
    semester       text not null,
    -- Shape-of-the-data version for this workspace, the successor of
    -- SemesterMeta.SchemaVersion. NOT the same thing as the goose version: goose
    -- owns the structure of the whole database, this owns what the rows mean.
    schema_version int  not null,
    -- Protects finished semesters from being written to.
    read_only      boolean not null default false,
    last_dump_at   timestamptz,
    created_at     timestamptz not null default now(),

    constraint semester_id_not_blank check (id <> '')
);

-- Which workspace the server is currently working in. Exactly one row.
--
-- This is the successor of the global `active_semester` document, and it is also
-- what replaces db.databaseName as the *stored* answer. The process-global
-- in-memory copy is unchanged -- de-globalising it is a separate project.
create table active_semester (
    id          int  primary key check (id = 1),
    semester_id text not null references semester(id) on delete restrict
);

-- Anchor and outcome of the nightly ZPA/Anny sync. Exactly one row.
--
-- semester_id is a log field, not a reference: it records which workspace the
-- last run touched and must survive that workspace being deleted.
-- last_finished and last_status are NULLABLE, and that is load-bearing.
-- TouchSchedulerFire writes the anchor BEFORE the run executes, precisely so a
-- crash cannot re-trigger the catch-up against a stale anchor. Between that write
-- and the first SaveSchedulerState there genuinely is no outcome yet -- NOT NULL
-- would have forced a fabricated one, and '' is not in the status check anyway.
-- db.SchedulerState keeps expressing it as the zero time and the empty string.
create table scheduler_state (
    id            int  primary key check (id = 1),
    -- Catch-up anchor: when the run was due, not when it finished.
    last_fire_at  timestamptz not null,
    last_finished timestamptz,
    last_status   text check (last_status in ('ok', 'errors', 'skipped', 'panic')),
    last_trigger  text not null check (last_trigger in ('nightly', 'catchup', 'manual')),
    semester_id   text,
    total_changes int  not null default 0
);

-- The editable per-semester configuration: planning period, allowed start times,
-- forbidden days, per-joint-program reserved times, email addresses and the
-- various gap/turnaround minutes.
--
-- jsonb, because it is always read and written as a whole and never queried by
-- field, and because its shape is genuinely a tree (JointProgramAllowedTimes is a
-- list of per-program time lists, Emails is a sub-document).
--
-- TRAP, and the reason format_version exists: model.SemesterConfigInput carries
-- MucDaiAllowedTimes with `json:"-"` and `bson:"mucDaiAllowedTimes"` -- a legacy
-- single MUC.DAI list that is STILL the live data (2026-WS has 30 entries there
-- and no JointProgramAllowedTimes; loadSemesterConfig seeds every joint program
-- from it). encoding/json would silently drop it. The data transfer must
-- materialise JointProgramAllowedTimes from the legacy list BEFORE writing this
-- column, after which the legacy field can be deleted from the model for good.
create table semester_config_input (
    semester_id    text primary key references semester(id) on delete cascade,
    config         jsonb not null,
    format_version int   not null default 1
);

-- Deliberately NOT created: the derived `semester_config` collection. It was
-- computed from semester_config_input, written with drop+insert on every load,
-- and never read back by anything (db/database.go:156 SaveSemesterConfig is its
-- only toucher). The derivation stays in Go; there is nothing to store.

-- +goose Down

drop table semester_config_input;
drop table scheduler_state;
drop table active_semester;
drop table semester;
