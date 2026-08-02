-- Exams and the planner's overlay on top of them.
--
-- The first semester-scoped slice: every table from here on carries semester_id,
-- and every foreign key between them is composite so that a join can never cross
-- a semester boundary.
--
-- The data-direction rule of this codebase -- what comes from ZPA is not modified
-- locally -- is expressed structurally: the ZPA-sourced facts live in `exam`, and
-- every local decision about an exam lives in its own table referencing it. An
-- import can therefore not clobber a planner decision even by accident.

-- +goose Up

-- Exams, both the ZPA-imported ones and the externally-owned ones (joint programs
-- and other faculties) that used to live apart in `non_zpaexams`.
--
-- One table with a `source` discriminator rather than two, because plan entries,
-- constraints and duration overrides all reference "an exam that exists" -- and
-- SQL cannot express a foreign key into the union of two tables. This is what
-- retires the hand-maintained knownAncodes set in plexams/validate_db.go:17 and
-- the "ancode is not a real exam" check at :117.
--
-- The ZPA import must UPSERT (insert ... on conflict do update), not drop+insert
-- as CacheZPAExams does today: the local overlay tables reference these rows, and
-- wiping the table would cascade the planner's decisions away with it.
--
-- And it must NOT delete the exams ZPA stopped delivering -- it sets withdrawn_at.
-- The asymmetry is the point: ZPA data can be re-imported at any time, the
-- planner's constraints cannot. A cascade from the volatile side to the
-- hand-entered side is backwards, and a single flaky import would destroy work.
-- Today those overlay rows survive as orphans and ValidateDBReferences reports
-- them; withdrawn_at keeps them AND says why they are orphaned.
create table exam (
    semester_id      text not null references semester(id) on delete cascade,
    ancode           int  not null,
    source           text not null check (source in ('zpa', 'external')),

    zpa_id           int,
    module           text not null,
    main_examer      text not null,
    main_examer_id   int  not null,
    exam_type        text not null,
    exam_type_full   text not null,
    -- ZPA's own date/time strings, frequently empty. The authoritative planned
    -- time lives on plan_entry, not here.
    zpa_date         text not null default '',
    zpa_starttime    text not null default '',
    duration_min     int  not null,
    is_repeater_exam boolean not null default false,
    -- Study group codes as ZPA emits them ("IF4B", ...). An array rather than a
    -- child table: they carry no referential meaning, they are parsed.
    groups           text[] not null default '{}',
    faculty          text not null default '',
    -- Set when ZPA stopped delivering this exam; cleared when it reappears. The
    -- row and everything hanging off it stay, so a cancelled -- or briefly
    -- missing -- exam never costs the planner their hand-entered constraints.
    -- Reads that drive planning filter on `withdrawn_at is null`.
    withdrawn_at     timestamptz,

    primary key (semester_id, ancode),
    constraint exam_duration_positive check (duration_min > 0)
);

-- ON DELETE CASCADE on the overlay tables below is still right, but only fires on
-- a deliberate deletion: dropping a whole semester, or removing an external exam
-- the planner created. The ZPA import must never be the thing that triggers it.

-- model.ZPAExam.Semester is deliberately NOT a column: it duplicated
-- semester.semester and could drift from it. Read it through the join.

-- The link from a ZPA exam to its Primuss counterparts, one row per (program,
-- primuss ancode). Was an array of sub-documents inside the exam.
--
-- `source` records where the link came from: 'zpa' for the ones ZPA delivers with
-- the exam, 'added' for the ones a planner adds by hand (the former
-- primuss_ancodes collection). Both are the same relationship.
create table exam_primuss_ancode (
    semester_id    text not null,
    ancode         int  not null,
    program        text not null references study_program(shortname),
    primuss_ancode int  not null,
    source         text not null check (source in ('zpa', 'added')),

    primary key (semester_id, ancode, program, primuss_ancode),
    foreign key (semester_id, ancode) references exam(semester_id, ancode) on delete cascade
);

-- Which exams this faculty plans. Absence of a row means UNDECIDED, which is a
-- real third state: the ZPA import auto-preselects only exams nobody has decided
-- about yet (autoPreselectExamsToPlan).
create table exam_to_plan (
    semester_id text    not null,
    ancode      int     not null,
    to_plan     boolean not null,

    primary key (semester_id, ancode),
    foreign key (semester_id, ancode) references exam(semester_id, ancode) on delete cascade
);

-- Per-exam planning constraints. The foreign key is what retires the orphan check
-- in plexams/validate_db.go ValidateDBConstraints.
--
-- exclude_days / possible_days stay arrays: they are plain date lists with no
-- referential meaning, always read and written together with the row.
create table exam_constraint (
    semester_id           text not null,
    ancode                int  not null,
    not_planned_by_me     boolean not null default false,
    -- Which other faculty plans it, when not_planned_by_me is set.
    not_planned_by_me_fk  text,
    do_not_publish        boolean not null default false,
    online                boolean not null default false,
    location              text,
    exclude_days          timestamptz[] not null default '{}',
    possible_days         timestamptz[] not null default '{}',
    fixed_day             timestamptz,
    fixed_time            timestamptz,

    primary key (semester_id, ancode),
    foreign key (semester_id, ancode) references exam(semester_id, ancode) on delete cascade,
    -- A fixed time implies a fixed day; the day is then redundant but harmless.
    constraint exam_constraint_fixed_day_or_time check (
        fixed_day is null or fixed_time is null or date(fixed_day) = date(fixed_time)
    )
);

-- Exams that must be scheduled at the same time. Stored as canonical unordered
-- pairs; the union-find over them happens in Go, as today.
create table exam_same_slot (
    semester_id  text not null,
    ancode       int  not null,
    other_ancode int  not null,

    primary key (semester_id, ancode, other_ancode),
    foreign key (semester_id, ancode)       references exam(semester_id, ancode) on delete cascade,
    foreign key (semester_id, other_ancode) references exam(semester_id, ancode) on delete cascade,
    -- One row per pair, never both directions, never a self-pair.
    constraint exam_same_slot_canonical check (ancode < other_ancode)
);

-- Room requirements of an exam. A separate table rather than nullable columns on
-- exam_constraint because model.Constraints.RoomConstraints is a POINTER: the
-- presence of the row is the presence of the constraint.
create table exam_room_constraint (
    semester_id        text not null,
    ancode             int  not null,
    places_with_socket boolean not null default false,
    lab                boolean not null default false,
    exahm              boolean not null default false,
    seb                boolean not null default false,
    kdp_jira_url       text,
    max_students       int,
    additional_seats   int,
    pre_exam_minutes   int,
    post_exam_minutes  int,
    comments           text,

    primary key (semester_id, ancode),
    foreign key (semester_id, ancode) references exam(semester_id, ancode) on delete cascade
);

-- The rooms an exam may use, if it is restricted to some. The reference to the
-- room master data is the point: a typo used to be a string nobody matched.
create table exam_allowed_room (
    semester_id text not null,
    ancode      int  not null,
    room_name   text not null references room(name),

    primary key (semester_id, ancode, room_name),
    foreign key (semester_id, ancode)
        references exam_room_constraint(semester_id, ancode) on delete cascade
);

-- Duration overriding the ZPA value. The foreign key retires the dangling-ancode
-- report in plexams/validate_db.go:383.
create table exam_duration_override (
    semester_id  text not null,
    ancode       int  not null,
    duration_min int  not null check (duration_min > 0),

    primary key (semester_id, ancode),
    foreign key (semester_id, ancode) references exam(semester_id, ancode) on delete cascade
);

-- Exam pairs that may share a start time despite a conflict. Both foreign keys
-- retire the check at plexams/validate_db.go:394.
create table exam_can_share_slot (
    semester_id text not null,
    ancode      int  not null,
    other_ancode int not null,

    primary key (semester_id, ancode, other_ancode),
    foreign key (semester_id, ancode)       references exam(semester_id, ancode) on delete cascade,
    foreign key (semester_id, other_ancode) references exam(semester_id, ancode) on delete cascade,
    constraint exam_can_share_slot_canonical check (ancode < other_ancode)
);

-- Per-student decisions about a conflict between two exams: ACCEPT drops that
-- student's proximity penalty, VETO forces it to count at full weight.
--
-- mtknr deliberately has NO foreign key: student registrations are Primuss source
-- data that gets replaced on every import, and a decision must outlive a
-- re-import. A decision about an unknown student is reported, not prevented.
create table exam_conflict_rating (
    semester_id  text not null,
    ancode       int  not null,
    other_ancode int  not null,
    mtknr        text not null,
    decision     text not null check (decision in ('ACCEPT', 'VETO')),

    primary key (semester_id, ancode, other_ancode, mtknr),
    foreign key (semester_id, ancode)       references exam(semester_id, ancode) on delete cascade,
    foreign key (semester_id, other_ancode) references exam(semester_id, ancode) on delete cascade,
    constraint exam_conflict_rating_canonical check (ancode < other_ancode)
);

create index exam_main_examer_idx on exam (semester_id, main_examer_id);
create index exam_primuss_ancode_lookup_idx
    on exam_primuss_ancode (semester_id, program, primuss_ancode);

-- +goose Down

drop table exam_conflict_rating;
drop table exam_can_share_slot;
drop table exam_duration_override;
drop table exam_allowed_room;
drop table exam_room_constraint;
drop table exam_same_slot;
drop table exam_constraint;
drop table exam_to_plan;
drop table exam_primuss_ancode;
drop table exam;
