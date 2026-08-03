-- The schedule and the room plan.
--
-- This is where the redundant starttime dies. rooms_planned carried its own copy
-- of the exam's start time, and nothing kept it in sync: SetExamTime
-- (plexams/plan.go:29) writes the plan entry and never touches the room plan, so
-- moving an exam left every one of its rooms pointing at the old time. A
-- hand-written detector looked for the damage afterwards
-- (plexams/validate_db.go:261). Here the column simply does not exist and the
-- time is read through a join.

-- +goose Up

-- When an exam is scheduled. A NULL starttime means "to be planned" -- the
-- distinction model.PlanEntry.IsPlanned() makes.
create table plan_entry (
    semester_id text not null,
    ancode      int  not null,
    starttime   timestamptz,
    -- The planner pinned this entry; the generator must not move it.
    locked      boolean not null default false,
    -- Frozen by phase A of the two-phase generation (EXaHM/SEB first).
    phase_fixed boolean not null default false,
    -- Planned by another faculty. Kept as a column rather than derived from
    -- exam.source: an exam can be ZPA-sourced and still be planned elsewhere
    -- (the notPlannedByMe constraint), so the two are not the same question.
    external    boolean not null default false,

    primary key (semester_id, ancode),
    foreign key (semester_id, ancode) references exam(semester_id, ancode) on delete cascade
);

create index plan_entry_starttime_idx on plan_entry (semester_id, starttime)
    where starttime is not null;

-- Which rooms an exam uses.
--
-- NO starttime column -- see planned_room_v below.
--
-- The natural key is (ancode, room, nta_mtknr), NOT (ancode, room): one physical
-- room legitimately holds several bookings for the same exam, because every NTA
-- student sitting there needs their own row for their own extended duration. A
-- real example from 2026-SS is ancode 225 in R1.046: one row of 43 students at 90
-- minutes plus three rows of one student each at 99 minutes. Checked against the
-- data before choosing this: (ancode, room) has 44 collisions, (ancode, room,
-- nta_mtknr) has none, in both 2026-SS and 2025-WS.
--
-- NULLS NOT DISTINCT is what makes the key work at all: the ordinary booking has
-- no nta_mtknr, and PostgreSQL's default would treat every NULL as unique and let
-- it be inserted twice.
create table planned_room (
    id                  bigint generated always as identity primary key,
    semester_id         text not null,
    ancode              int  not null,
    room_name           text not null references room(name),
    -- Longer than the exam for NTA bookings (time compensation).
    --
    -- >= 0, not > 0: the value is the exam's MaxDuration copied verbatim by the
    -- room generator (plexams/roomplan_build.go:388), and an exam whose duration
    -- nobody has set yet contributes a 0. 2026-SS has one such row -- ancode 543
    -- in R2.007, pre-planned, eleven students, not a reserve room. A `> 0` check
    -- would not have caught bad data; it would have made room generation fail on
    -- data the planner considers fine.
    duration_min        int  not null check (duration_min >= 0),
    handicap            boolean not null default false,
    handicap_room_alone boolean not null default false,
    reserve             boolean not null default false,
    nta_mtknr           text references nta(mtknr),
    pre_planned         boolean not null default false,

    foreign key (semester_id, ancode) references plan_entry(semester_id, ancode) on delete cascade,
    constraint planned_room_one_booking_per_room_and_nta
        unique nulls not distinct (semester_id, ancode, room_name, nta_mtknr)
);

-- Who sits in a room. Was an array inside the room document; as a table the
-- primary key makes "the same student seated twice for one exam" impossible,
-- which plexams/validate_db.go:296 had to look for.
create table planned_room_student (
    planned_room_id bigint not null references planned_room(id) on delete cascade,
    mtknr           text   not null,

    primary key (planned_room_id, mtknr)
);

-- The starttime the room plan used to store. Reading it through the exam's plan
-- entry means it cannot be stale, so there is nothing left for the
-- "stale after a move?" detector to find.
create view planned_room_v as
    select pr.*, pe.starttime
    from planned_room pr
    join plan_entry pe using (semester_id, ancode);

-- Students who did not fit into any room. Also had a redundant starttime.
-- mtknrs stays an array: a scratch list with no referential meaning, always read
-- and written whole.
create table unplaced_exam (
    semester_id text not null,
    ancode      int  not null,
    mtknrs      text[] not null default '{}',
    nta_mtknr   text references nta(mtknr),

    primary key (semester_id, ancode),
    foreign key (semester_id, ancode) references plan_entry(semester_id, ancode) on delete cascade
);

-- Rooms unavailable at a given time. starttime IS kept here: it is the block's
-- own time, not a copy of an exam's.
create table blocked_room (
    semester_id text not null references semester(id) on delete cascade,
    room_name   text not null references room(name),
    starttime   timestamptz not null,
    reason      text,

    primary key (semester_id, room_name, starttime)
);

-- Rooms requested from Gebäudemanagement. valid_from/valid_until are the
-- request's own window; starttime relates it to the exam slot it was requested
-- for, and is what identifies the request.
--
-- The key is (room, starttime), not (room, valid_from): every db method addresses
-- a request that way -- GetRoomRequest, SetRoomRequestApproved/Active,
-- UpdateRoomRequestTime -- and UpdateRoomRequestTime *changes* valid_from, so a
-- key containing it would have moved the row out from under the caller that just
-- edited it. The two really are different values: in Test26SS-v2 all 57 requests
-- have valid_from 15 minutes before starttime, the setup buffer.
--
-- starttime is therefore NOT NULL. Every construction site sets it
-- (plexams/room_requests.go:75,101, room_requests_generate.go:194,
-- csv_export.go:732); a row without one could be neither approved nor deleted.
create table room_request (
    semester_id text not null references semester(id) on delete cascade,
    room_name   text not null references room(name),
    starttime   timestamptz not null,
    valid_from  timestamptz not null,
    valid_until timestamptz not null,
    approved    boolean not null default false,
    active      boolean not null default true,

    primary key (semester_id, room_name, starttime),
    constraint room_request_window_ordered check (valid_until > valid_from)
);

-- Rooms the planner assigned by hand before generation. Hand-entered, so it hangs
-- off exam rather than plan_entry: it exists before anything is scheduled.
--
-- The key is (ancode, room, mtknr), not (ancode, room) -- the same shape as
-- planned_room above, and for the same reason: the planner pre-assigns a room for
-- the exam as a whole (mtknr NULL) and additionally pins individual NTA students
-- into rooms of their own. AddPrePlannedRoomToExam / RemovePrePlannedRoomFromExam
-- (db/rooms.go) have always addressed the document by all three fields. Checked
-- against the data before fixing this: in 2026-SS ancode 355 has four rows in
-- T3.017 (three mtknrs plus the NULL one) and 2025-WS has the same pattern, so
-- (semester_id, ancode, room_name) would have rejected the real room plan on
-- import.
--
-- NULLS NOT DISTINCT for the same reason as planned_room: without it the row with
-- no mtknr could be inserted twice.
create table pre_planned_room (
    id          bigint generated always as identity primary key,
    semester_id text not null,
    ancode      int  not null,
    room_name   text not null references room(name),
    mtknr       text,
    reserve     boolean not null default false,
    seats       int,

    foreign key (semester_id, ancode) references exam(semester_id, ancode) on delete cascade,
    constraint pre_planned_room_one_per_room_and_student
        unique nulls not distinct (semester_id, ancode, room_name, mtknr)
);

-- Exemptions from an NTA's "needs a room alone" entitlement, for one exam.
create table nta_room_alone_waiver (
    semester_id text not null,
    ancode      int  not null,
    mtknr       text not null references nta(mtknr),
    reason      text not null,

    primary key (semester_id, ancode, mtknr),
    foreign key (semester_id, ancode) references exam(semester_id, ancode) on delete cascade
);

-- Vorplanung: the manual pseudo-exams (EXaHM/SEB) that size the Anny bookings
-- before the real ZPA exams are known.
--
-- id stays an explicit per-semester integer rather than a global identity: the
-- pair tables below reference it, and the CSV round-trip re-imports rows by id so
-- those references survive a restore (UpsertPreplanExam exists for exactly that).
create table preplan_exam (
    semester_id       text not null references semester(id) on delete cascade,
    id                int  not null,
    exam_kind         text not null check (exam_kind in ('EXaHM', 'SEB')),
    examer_id         int  not null,
    examer_name       text not null default '',
    module            text not null default '',
    programs          text[] not null default '{}',
    expected_students int  not null default 0,
    duration_min      int,
    planned_starttime timestamptz,
    is_fixed          boolean not null default false,
    -- The linked real ZPA exam, once it is known.
    ancode            int,
    notes             text not null default '',
    -- The embedded model.Constraints, if any. jsonb because it is an optional
    -- sub-document read and written with its row, and because duplicating the
    -- exam_constraint tables for a different parent would buy nothing.
    constraints       jsonb,
    format_version    int  not null default 1,

    primary key (semester_id, id)
);

-- Preplan exams that must not share a slot, and ones that may. Stored as
-- canonical unordered pairs of preplan ids, mirroring exam_same_slot.
create table preplan_not_same_slot (
    semester_id text not null,
    id          int  not null,
    other_id    int  not null,

    primary key (semester_id, id, other_id),
    foreign key (semester_id, id)       references preplan_exam(semester_id, id) on delete cascade,
    foreign key (semester_id, other_id) references preplan_exam(semester_id, id) on delete cascade,
    constraint preplan_not_same_slot_canonical check (id < other_id)
);

create table preplan_can_share_slot (
    semester_id text not null,
    id          int  not null,
    other_id    int  not null,

    primary key (semester_id, id, other_id),
    foreign key (semester_id, id)       references preplan_exam(semester_id, id) on delete cascade,
    foreign key (semester_id, other_id) references preplan_exam(semester_id, id) on delete cascade,
    constraint preplan_can_share_slot_canonical check (id < other_id)
);

-- +goose Down

drop table preplan_can_share_slot;
drop table preplan_not_same_slot;
drop table preplan_exam;
drop table nta_room_alone_waiver;
drop table pre_planned_room;
drop table room_request;
drop table blocked_room;
drop table unplaced_exam;
drop view planned_room_v;
drop table planned_room_student;
drop table planned_room;
drop table plan_entry;
