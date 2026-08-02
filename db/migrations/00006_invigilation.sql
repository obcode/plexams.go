-- Aufsichten.
--
-- No foreign key here points at a teacher. Teachers are a ZPA cache that gets
-- replaced on every import, and invigilator_constraint is hand-entered -- the
-- same asymmetry that made exam withdrawal a marking rather than a deletion. A
-- constraint for someone who has left the pool is reported, never silently
-- dropped.

-- +goose Up

-- One table for what were invigilations_self and invigilations_other: the
-- distinction is a property of the row (model.Invigilation.IsSelfInvigilation),
-- not of a collection. Replacing "all self invigilations" becomes a delete with a
-- predicate, which it effectively already was.
--
-- starttime is kept: an invigilation covers a room during a slot, and several
-- exams can sit in that slot. It is not a copy of one exam's time, unlike the one
-- that was removed from the room plan.
create table invigilation (
    id                   bigint generated always as identity primary key,
    semester_id          text not null references semester(id) on delete cascade,
    invigilator_id       int  not null,
    starttime            timestamptz not null,
    -- Null for a reserve duty that is not tied to one room.
    room_name            text references room(name),
    duration_min         int  not null check (duration_min > 0),
    is_reserve           boolean not null default false,
    is_self_invigilation boolean not null default false,
    pre_planned          boolean not null default false
);

create index invigilation_by_invigilator_idx on invigilation (semester_id, invigilator_id, starttime);
create index invigilation_by_time_idx on invigilation (semester_id, starttime);

-- Duties the planner assigned by hand before generation. Hand-entered, so the
-- generator must read it rather than overwrite it.
create table pre_planned_invigilation (
    semester_id    text not null references semester(id) on delete cascade,
    invigilator_id int  not null,
    starttime      timestamptz not null,
    room_name      text references room(name),
    is_reserve     boolean not null default false,

    primary key (semester_id, invigilator_id, starttime)
);

-- What ZPA says about an invigilator's availability and workload. Source data,
-- replaced on every import.
--
-- excluded_dates stays an array of TEXT because that is what ZPA delivers:
-- "02.01.06" strings, parsed on read. Turning them into dates here would move a
-- parsing decision into the schema and hide the format the source actually uses.
create table invigilator_requirement (
    semester_id              text not null references semester(id) on delete cascade,
    invigilator_id           int  not null,
    invigilator              text not null default '',
    excluded_dates           text[] not null default '{}',
    part_time                double precision not null default 1,
    oral_exams_contribution  int  not null default 0,
    livecoding_contribution  int  not null default 0,
    master_contribution      int  not null default 0,
    free_semester            double precision not null default 0,
    overtime_last_semester   double precision not null default 0,
    overtime_this_semester   double precision not null default 0,

    primary key (semester_id, invigilator_id)
);

-- The planner's additions on top of the ZPA requirements, edited in the GUI.
create table invigilator_constraint (
    semester_id        text not null references semester(id) on delete cascade,
    teacher_id         int  not null,
    is_not_invigilator boolean not null default false,

    primary key (semester_id, teacher_id)
);

create table invigilator_excluded_date (
    semester_id text not null,
    teacher_id  int  not null,
    excluded_on timestamptz not null,

    primary key (semester_id, teacher_id, excluded_on),
    foreign key (semester_id, teacher_id)
        references invigilator_constraint(semester_id, teacher_id) on delete cascade
);

-- Windows on a day in which the invigilator is (un)available. from/until are
-- nullable: a window may be open-ended on either side.
create table invigilator_time_window (
    semester_id  text not null,
    teacher_id   int  not null,
    window_date  timestamptz not null,
    available_from  timestamptz,
    available_until timestamptz,

    primary key (semester_id, teacher_id, window_date),
    foreign key (semester_id, teacher_id)
        references invigilator_constraint(semester_id, teacher_id) on delete cascade,
    constraint invigilator_time_window_ordered check (
        available_from is null or available_until is null or available_until > available_from
    )
);

-- The computed fair-share summary: totals, the per-invigilator target and the
-- resolved invigilator list. Derived from everything above by
-- PrepareInvigilationTodos, so jsonb -- normalising a cache of data that already
-- exists relationally buys nothing.
--
-- Was a single document with a fixed _id "todos"; here it is one row per semester,
-- which is what it always meant.
create table invigilation_todos (
    semester_id    text primary key references semester(id) on delete cascade,
    todos          jsonb not null,
    format_version int   not null default 1
);

-- +goose Down

drop table invigilation_todos;
drop table invigilator_time_window;
drop table invigilator_excluded_date;
drop table invigilator_constraint;
drop table invigilator_requirement;
drop table pre_planned_invigilation;
drop table invigilation;
