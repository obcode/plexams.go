-- Todos: the planners' own task list, created and worked off in the GUI.
--
-- A todo belongs to one semester like everything else; carrying open (and
-- recurring) todos into the next semester is an explicit copy, recorded in
-- carried_from_*, so pressing the button twice copies nothing twice.
--
-- Links point at phases, conditions, exams, teachers, rooms, ... by (kind, key)
-- WITHOUT a foreign key to the target, on purpose:
--   * phases and planning conditions are Go constants, not rows;
--   * teachers are deleted and re-inserted on every ZPA import, so an FK would
--     either block the import or cascade the links away;
--   * rooms, study programs and NTAs are global, exams semester-scoped -- one
--     column cannot reference all of them.
-- The key is validated in Go when the link is added, and the display label is
-- resolved on read, so a link to something that vanished later still shows its key.

-- +goose Up

create table todo (
    semester_id           text    not null references semester(id) on delete cascade,
    id                    bigint  generated always as identity primary key,
    title                 text    not null check (title <> ''),
    -- Markdown, rendered (and sanitised) by the GUI.
    description           text    not null default '',
    priority              text    not null default 'NORMAL'
                                  check (priority in ('LOW', 'NORMAL', 'HIGH')),
    due_date              date,
    labels                text[]  not null default '{}',
    -- Comes back (open) in every following semester on carry-over.
    recurring             boolean not null default false,
    done_at               timestamptz,
    -- Email of the user, like mutation_log.user_email.
    done_by               text,
    created_at            timestamptz not null default now(),
    created_by            text    not null,
    updated_at            timestamptz not null default now(),
    carried_from_semester text,
    carried_from_id       bigint,

    -- Composite target for the child tables' semester-carrying foreign keys.
    unique (semester_id, id),
    constraint todo_done_identity check (num_nonnulls(done_at, done_by) in (0, 2)),
    constraint todo_carried_from check (num_nonnulls(carried_from_semester, carried_from_id) in (0, 2))
);

create index todo_open_idx on todo (semester_id, done_at);
create index todo_labels_idx on todo using gin (labels);
create unique index todo_carried_from_idx on todo (semester_id, carried_from_semester, carried_from_id);

create table todo_comment (
    semester_id text   not null,
    todo_id     bigint not null,
    id          bigint generated always as identity primary key,
    -- Markdown, like todo.description.
    body        text   not null check (body <> ''),
    author      text   not null,
    created_at  timestamptz not null default now(),
    edited_at   timestamptz,

    foreign key (semester_id, todo_id) references todo(semester_id, id) on delete cascade
);

create index todo_comment_todo_idx on todo_comment (semester_id, todo_id);

create table todo_link (
    semester_id text   not null,
    todo_id     bigint not null,
    kind        text   not null check (kind in ('PHASE', 'CONDITION', 'EXAM', 'TEACHER', 'ROOM',
                                                'STUDY_PROGRAM', 'DAY', 'NTA', 'URL', 'JIRA')),
    key         text   not null check (key <> ''),

    primary key (semester_id, todo_id, kind, key),
    foreign key (semester_id, todo_id) references todo(semester_id, id) on delete cascade
);

-- The reverse lookup: which todos hang off this exam / phase / ...
create index todo_link_target_idx on todo_link (semester_id, kind, key);

-- +goose Down

drop table todo_link;
drop table todo_comment;
drop table todo;
