-- Inquiries of the examiner portal: the planners ask, the examiners answer.
--
-- One inquiry per semester and kind. An examiner's answer is a submission with an
-- append-only revision history; the planning tables (preplan_exam, ...) stay the
-- planners' decision and are only written by an explicit apply step. Every change made
-- through the portal also lands in inquiry_activity, in the same transaction, which
-- feeds the planners' overview and their notification mails.
--
-- No foreign key to teacher or exam: the teacher mirror is refilled on every ZPA import
-- and exams are cleaned up by the joint import. ZPA person ids and ancodes are stable
-- values and validated in Go.

-- +goose Up

create table inquiry (
    semester_id text not null references semester(id) on delete cascade,
    kind        text not null check (kind in
                    ('PREPLAN', 'EXAMS_TO_PLAN', 'CONSTRAINTS', 'DRAFT_FEEDBACK', 'INVIGILATION')),
    state       text not null default 'DRAFT' check (state in ('DRAFT', 'OPEN', 'CLOSED')),
    deadline    timestamptz,
    -- Markdown shown above the form; rendered sanitised by the GUI.
    intro       text not null default '',
    opened_at   timestamptz,
    opened_by   text,
    closed_at   timestamptz,
    closed_by   text,
    updated_at  timestamptz not null default now(),

    primary key (semester_id, kind)
);

-- Per examiner and inquiry: invitation, reminders, "nothing to report".
create table inquiry_response (
    semester_id          text not null references semester(id) on delete cascade,
    kind                 text not null,
    teacher_id           int  not null check (teacher_id > 0),
    invited_at           timestamptz,
    reminded_at          timestamptz,
    reminder_count       int  not null default 0,
    nothing_to_report_at timestamptz,
    nothing_to_report_by text,

    primary key (semester_id, kind, teacher_id),
    foreign key (semester_id, kind) references inquiry(semester_id, kind) on delete cascade
);

-- Head row of a submission: identity and workflow state. The content is in the
-- revisions.
create table inquiry_submission (
    semester_id      text not null references semester(id) on delete cascade,
    id               bigint generated always as identity primary key,
    kind             text not null,
    -- Whom the statement is about.
    teacher_id       int  not null check (teacher_id > 0),
    teacher_name     text not null default '',
    -- The login that created it -- may differ from the teacher (a prof reporting for a
    -- lecturer, a delegate).
    created_by       text not null,
    -- EXAMS_TO_PLAN, CONSTRAINTS, DRAFT_FEEDBACK: the exam it is about.
    ancode           int,
    -- PREPLAN: the pre-exam it became. Set null when the planners delete it; such a
    -- detached submission is never re-created automatically.
    preplan_exam_id  int,
    state            text not null default 'SUBMITTED' check (state in ('SUBMITTED', 'WITHDRAWN')),
    revision         int  not null default 1 check (revision >= 1),
    applied_revision int,
    review_state     text not null default 'OPEN' check (review_state in
                         ('OPEN', 'APPLIED', 'APPLIED_MODIFIED', 'REJECTED', 'DONE')),
    reviewed_revision int,
    reviewed_by      text,
    reviewed_at      timestamptz,
    review_note      text not null default '',
    created_at       timestamptz not null default now(),
    updated_at       timestamptz not null default now(),

    unique (semester_id, id),
    foreign key (semester_id, kind) references inquiry(semester_id, kind) on delete cascade,
    foreign key (semester_id, preplan_exam_id) references preplan_exam(semester_id, id)
        on delete set null (preplan_exam_id)
);

create unique index inquiry_submission_preplan_idx
    on inquiry_submission (semester_id, preplan_exam_id) where preplan_exam_id is not null;
create unique index inquiry_submission_ancode_idx
    on inquiry_submission (semester_id, kind, ancode) where ancode is not null;
create unique index inquiry_submission_invigilation_idx
    on inquiry_submission (semester_id, teacher_id) where kind = 'INVIGILATION';
create index inquiry_submission_teacher_idx on inquiry_submission (semester_id, kind, teacher_id);
create index inquiry_submission_inbox_idx on inquiry_submission (semester_id, kind, review_state);

-- The content of a submission, append-only.
create table inquiry_submission_revision (
    semester_id    text   not null,
    submission_id  bigint not null,
    revision       int    not null check (revision >= 1),
    state          text   not null check (state in ('SUBMITTED', 'WITHDRAWN')),
    -- Hand-written Go type per kind, versioned by format_version.
    payload        jsonb  not null,
    format_version int    not null default 1,
    submitted_by   text   not null,
    channel        text   not null default 'WEB' check (channel in ('WEB', 'TOKEN')),
    submitted_at   timestamptz not null default now(),

    primary key (semester_id, submission_id, revision),
    foreign key (semester_id, submission_id)
        references inquiry_submission(semester_id, id) on delete cascade
);

-- One row per change made through the portal. Append-only; the planners' overview and
-- the notification mails read it.
create table inquiry_activity (
    semester_id   text not null references semester(id) on delete cascade,
    id            bigint generated always as identity primary key,
    at            timestamptz not null default now(),
    actor         text not null,
    channel       text not null default 'WEB' check (channel in ('WEB', 'TOKEN')),
    teacher_id    int  not null,
    teacher_name  text not null default '',
    kind          text not null,
    action        text not null check (action in
                      ('SUBMITTED', 'CHANGED', 'WITHDRAWN', 'NOTHING_TO_REPORT', 'REVOKED')),
    submission_id bigint,
    revision      int,
    -- Short human label, e.g. the module.
    label         text not null default '',
    -- [{"field": ..., "old": ..., "new": ...}]
    changes       jsonb not null default '[]',

    unique (semester_id, id),
    foreign key (semester_id, submission_id)
        references inquiry_submission(semester_id, id) on delete cascade
);

create index inquiry_activity_at_idx on inquiry_activity (at desc);

-- How a planner wants to hear about portal activity. Global; one row per app_user.
create table notification_setting (
    email        text primary key references app_user(email) on delete cascade,
    mode         text not null default 'DAILY' check (mode in ('OFF', 'IMMEDIATE', 'DAILY')),
    -- The last inquiry_activity.id mailed / seen in the overview.
    mailed_up_to bigint not null default 0,
    seen_up_to   bigint not null default 0,
    updated_at   timestamptz not null default now()
);

-- +goose Down

drop table notification_setting;
drop table inquiry_activity;
drop table inquiry_submission_revision;
drop table inquiry_submission;
drop table inquiry_response;
drop table inquiry;
