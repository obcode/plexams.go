-- The remaining per-semester tables: the ZPA caches, Anny bookings, the planning
-- state machine, the audit logs, the assembled-exam cache and a few odds and ends.
--
-- Rule of thumb applied throughout: relational where a constraint or a query
-- predicate hangs on it, jsonb where the value is a document that is always read
-- and written whole. Every jsonb column carries a format_version so a renamed Go
-- field cannot be misread from an old blob in silence.

-- +goose Up

-- Teachers, mirrored from ZPA. Source data, replaced on import.
create table teacher (
    semester_id   text not null references semester(id) on delete cascade,
    id            int  not null,
    shortname     text not null default '',
    fullname      text not null default '',
    email         text not null default '',
    is_prof       boolean not null default false,
    is_lba        boolean not null default false,
    is_prof_hc    boolean not null default false,
    is_staff      boolean not null default false,
    is_active     boolean not null default true,
    last_semester text not null default '',
    fk            text not null default '',

    primary key (semester_id, id)
);

-- GetTeacherByEmail matches case-insensitively (a login address may be cased
-- differently from the ZPA record), so the index has to be on lower(email) or it
-- will not be used.
create index teacher_email_idx on teacher (semester_id, lower(email));
create index teacher_fullname_idx on teacher (semester_id, lower(fullname));

-- Students, mirrored from ZPA.
create table zpa_student (
    semester_id text not null references semester(id) on delete cascade,
    mtknr       text not null,
    greeting    text not null default '',
    first_name  text not null default '',
    last_name   text not null default '',
    email       text not null default '',
    gender      text not null default '',
    group_name  text not null default '',

    primary key (semester_id, mtknr)
);

-- Room bookings fetched from Anny. Source data, re-fetchable at any time.
--
-- status_reason and resource_id carry json:"-" in the model: they are stored but
-- not part of the GraphQL contract. As columns they simply survive -- as jsonb
-- they would have been dropped by encoding/json, which is the trap
-- TestJSONBTypesLoseNoField exists for. `mine` is the opposite case (bson:"-"):
-- computed on read, so it is not a column at all.
create table anny_booking (
    semester_id              text not null references semester(id) on delete cascade,
    number                   text not null,
    start_date               timestamptz not null,
    end_date                 timestamptz not null,
    blocker_start_date       timestamptz not null,
    blocker_end_date         timestamptz not null,
    charged_duration         int  not null default 0,
    description              text not null default '',
    note                     text not null default '',
    room                     text not null default '',
    status                   text not null default '',
    status_reason            jsonb,
    is_blocker               boolean not null default false,
    can_edit                 boolean not null default false,
    is_editable              boolean not null default false,
    manually_created         boolean not null default false,
    has_custom_description   boolean not null default false,
    self_url                 text not null default '',
    personalization_name     text not null default '',
    booking_group_identifier text not null default '',
    resource_id              text not null default '',
    created_at               timestamptz,
    updated_at               timestamptz,
    canceled_at              timestamptz,
    cancelable_until         timestamptz,

    primary key (semester_id, number),
    constraint anny_booking_window_ordered check (end_date >= start_date)
);

create index anny_booking_by_time_idx on anny_booking (semester_id, start_date);

-- Which conditions of the planning Petri net currently hold. A set of keys.
create table planning_state (
    semester_id   text not null references semester(id) on delete cascade,
    condition_key text not null,

    primary key (semester_id, condition_key)
);

-- Audit trail: one row per GraphQL mutation.
--
-- args is jsonb (a list of key/value pairs, with secrets masked before it gets
-- here). ancodes is an int[] because the Mongo query matched it by bare equality
-- against the array -- that becomes `$1 = any(ancodes)`.
create table mutation_log (
    id          bigint generated always as identity primary key,
    semester_id text not null references semester(id) on delete cascade,
    logged_at   timestamptz not null default now(),
    name        text not null,
    type        text not null,
    user_email  text,
    args        jsonb not null default '[]',
    ancodes     int[] not null default '{}',
    error       text,
    duration_ms int  not null default 0
);

create index mutation_log_recent_idx on mutation_log (semester_id, logged_at desc);
create index mutation_log_ancodes_idx on mutation_log using gin (ancodes);

-- Import/upload history, one row per sync run, with the per-item diff.
create table sync_log (
    id          bigint generated always as identity primary key,
    semester_id text not null references semester(id) on delete cascade,
    logged_at   timestamptz not null default now(),
    operation   text not null,
    label       text not null default '',
    direction   text not null check (direction in ('import', 'upload')),
    system      text not null,
    ok          boolean not null,
    summary     text not null default '',
    added       int not null default 0,
    changed     int not null default 0,
    removed     int not null default 0,
    -- The per-entry field-level diff. A nested list of lists, read and written
    -- with its row and never queried into.
    entries     jsonb not null default '[]',
    format_version int not null default 1
);

create index sync_log_recent_idx on sync_log (semester_id, logged_at desc);

-- The denormalised exam the planning actually works on: the ZPA exam, its Primuss
-- exams with their registrations, conflicts, NTAs and constraints, four levels
-- deep. A materialised cache, rebuilt from the tables it was assembled from --
-- so jsonb. Normalising a copy of data that already exists relationally would buy
-- nothing and cost a second source of truth.
create table assembled_exam (
    semester_id    text not null,
    ancode         int  not null,
    exam           jsonb not null,
    format_version int  not null default 1,

    primary key (semester_id, ancode),
    foreign key (semester_id, ancode) references exam(semester_id, ancode) on delete cascade
);

-- Whether the cache above needs rebuilding. One row per semester.
create table assembled_exams_state (
    semester_id text primary key references semester(id) on delete cascade,
    dirty       boolean not null default true
);

-- Files uploaded for a mail (cover pages, invigilator plans). The bytes live in
-- the row, as they did in the document.
create table email_attachment (
    semester_id  text not null references semester(id) on delete cascade,
    kind         text not null,
    key          text not null,
    filename     text not null,
    content_type text not null default '',
    size         int  not null,
    data         bytea not null,
    uploaded_at  timestamptz not null default now(),

    primary key (semester_id, kind, key),
    constraint email_attachment_size_matches check (size = length(data))
);

-- Exams added outside the regular plan, with their rooms. date/time stay TEXT:
-- they are entered as strings and parsed on use, like ZPA's own date fields.
create table additional_exam (
    semester_id text not null,
    ancode      int  not null,
    exam_date   text not null default '',
    exam_time   text not null default '',

    primary key (semester_id, ancode),
    foreign key (semester_id, ancode) references exam(semester_id, ancode) on delete cascade
);

create table additional_exam_room (
    semester_id    text not null,
    ancode         int  not null,
    room_name      text not null references room(name),
    invigilator_id int  not null,
    duration_min   int  not null check (duration_min > 0),
    is_reserve     boolean not null default false,
    is_handicap    boolean not null default false,
    student_count  int  not null default 0,

    primary key (semester_id, ancode, room_name),
    foreign key (semester_id, ancode) references additional_exam(semester_id, ancode) on delete cascade
);

-- Named groups of exams for the special-interest reports. ancodes stays an array:
-- a display grouping with no referential weight -- a stale entry costs a missing
-- line in a report, not a wrong plan.
create table special_interest (
    semester_id text not null references semester(id) on delete cascade,
    name        text not null,
    filename    text not null default '',
    ancodes     int[] not null default '{}',

    primary key (semester_id, name)
);

-- Registrations ZPA refused on upload, with its reason. A rejection record, kept
-- for the planner to act on; jsonb because both halves are ZPA's own payloads.
create table studentreg_upload_error (
    id           bigint generated always as identity primary key,
    semester_id  text not null references semester(id) on delete cascade,
    registration jsonb not null,
    error        jsonb not null,
    format_version int not null default 1
);

-- +goose Down

drop table studentreg_upload_error;
drop table special_interest;
drop table additional_exam_room;
drop table additional_exam;
drop table email_attachment;
drop table assembled_exams_state;
drop table assembled_exam;
drop table sync_log;
drop table mutation_log;
drop table planning_state;
drop table anny_booking;
drop table zpa_student;
drop table teacher;
