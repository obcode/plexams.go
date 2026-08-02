-- Global master data: semester-independent entities that carry over between
-- planning cycles. In MongoDB these lived in the separate `plexams` database;
-- here they are simply the tables without a semester_id column.
--
-- Note on derived fields: several model structs carry values that are computed,
-- not stored (model.Planer.Effective*/DefaultMail, model.Room.NeedsRequest).
-- Those are NOT columns -- they stay computed in Go, or become generated columns
-- where SQL can do it. Storing a derived value is exactly what let the two
-- spellings of `needsRequest` drift apart in the Mongo data.

-- +goose Up

-- Nachteilsausgleich. Keyed by Matrikelnummer, which is TEXT and never numeric:
-- the numbers have leading zeros.
--
-- valid_from / valid_until are TEXT on purpose. They are free-form semester
-- labels in the model (model.NTA.From/Until), not dates -- do not "improve"
-- them into date columns without changing the model first.
create table nta (
    mtknr                  text    primary key,
    name                   text    not null,
    email                  text,
    compensation           text    not null,
    delta_duration_percent int     not null,
    needs_room_alone       boolean not null default false,
    needs_hardware         boolean not null default false,
    program                text    not null,
    valid_from             text    not null,
    valid_until            text    not null,
    last_semester          text,
    deactivated            boolean not null default false
);

-- Rooms. The name is the key everywhere in the system: planned rooms, blocked
-- rooms and room requests all reference it by name, not by a surrogate id.
--
-- needs_request is GENERATED. model.Room documents it as derived from
-- request_with, but Mongo stored it anyway -- under two different keys
-- (`needsRequest` and `needsrequest`), because model.Room has no bson tags and
-- the driver lowercases Go field names. Here the derivation IS the definition
-- and cannot drift.
create table room (
    name               text    primary key,
    seats              int     not null check (seats >= 0),
    handicap           boolean not null default false,
    lab                boolean not null default false,
    places_with_socket boolean not null default false,
    request_with       text    not null default 'NONE'
                       check (request_with in ('NONE', 'ANNY', 'MANAGEMENT')),
    needs_request      boolean not null generated always as (request_with <> 'NONE') stored,
    -- Lower number = preferred when generating room requests.
    request_priority   int     not null default 0,
    exahm              boolean not null default false,
    seb                boolean not null default false,
    seb_seats          int,
    hmeb_seats         int,
    deactivated        boolean not null default false,
    -- Optional summer heat override; null = derive from the R-building floor.
    hitzewert          int
);

-- Study programs. shortname is the internal key and may carry a degree suffix
-- (DC-B / DC-M) because ZPA's two-letter codes are not unique; zpa_code maps
-- back at the ZPA boundary and defaults to shortname when empty (IS-M today).
create table study_program (
    shortname           text    primary key,
    name                text    not null,
    degree              text,
    zpa_code            text    not null default '',
    category            text    not null check (category in ('fk07', 'joint', 'misc')),
    active              boolean not null default true,
    retired             boolean not null default false,
    -- Base ancode for externally imported exams: local ancode = base + primuss ancode.
    external_exams_base int,
    -- Set exactly for joint programs (MUC.DAI, MUC.HEALTH), null otherwise.
    joint_faculty       text,

    constraint study_program_joint_faculty_iff_joint
        check ((category = 'joint') = (joint_faculty is not null))
);

-- Teachers permanently exempt from invigilation. valid_from/valid_until are
-- semester labels (null = always), so retiring an exemption keeps past semesters
-- correct instead of deleting the history.
create table permanent_non_invigilator (
    teacher_id  int  primary key,
    -- Denormalized on purpose: the entry must stay readable after the teacher has
    -- left the invigilator pool and can no longer be resolved to a name.
    name        text not null,
    reason      text not null,
    valid_from  text,
    valid_until text
);

-- Login allow-list. Authorization is enforced in the backend against this table;
-- absence means no access (fail-closed). "user" is a reserved word in SQL.
create table app_user (
    email     text primary key,
    name      text not null,
    role      text not null check (role in ('ADMIN', 'PLANER', 'VIEWER')),
    -- Kürzel override; empty = fall back to the ZPA teacher shortname.
    shortname text not null default ''
);

-- Per-user encrypted personal access tokens (AES-256-GCM, sealed in Go -- the key
-- never touches the database). Kept 1:1 with db.UserSecret rather than
-- generalised to (email, kind): the planned ZPA token can have that when it
-- arrives, together with the Go-side change.
create table user_secret (
    email            text primary key,
    jira_key_version int,
    jira_nonce       bytea,
    jira_ciphertext  bytea,
    jira_updated_at  timestamptz,

    -- A sealed value is all-or-nothing; a half-written secret cannot be opened.
    constraint user_secret_jira_complete check (
        num_nonnulls(jira_key_version, jira_nonce, jira_ciphertext) in (0, 3)
    )
);

-- Markdown overrides for the built-in email templates. A missing row means the
-- compiled-in default is used.
create table email_template (
    name     text primary key,
    markdown text not null
);

-- The shared sender identity for outgoing mail. Exactly one row. The Effective*
-- and defaultMail fields of model.Planer are derived at read time and are
-- deliberately absent here.
create table planer (
    id           int  primary key check (id = 1),
    name         text not null,
    email        text not null,
    test_mail    text,
    cc           text,
    noreply_mail text,
    noreply_name text
);

-- Names whose Anny bookings count as ours. Exactly one row.
create table anny_config (
    id                    int    primary key check (id = 1),
    personalization_names text[] not null default '{}'
);

-- Tuning knobs for the Terminplan generator: ~33 numeric weights that are always
-- read and written as a whole and never queried by field. jsonb keeps them out of
-- the schema, so adding a weight stays a GUI change instead of a migration.
create table generation_config (
    id             int   primary key check (id = 1),
    config         jsonb not null,
    -- Bumped when the Go struct changes shape incompatibly, so an old blob cannot
    -- be read silently into a renamed field.
    format_version int   not null default 1
);

-- +goose Down

drop table generation_config;
drop table anny_config;
drop table planer;
drop table email_template;
drop table user_secret;
drop table app_user;
drop table permanent_non_invigilator;
drop table study_program;
drop table room;
drop table nta;
