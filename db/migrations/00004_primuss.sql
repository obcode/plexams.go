-- Primuss: registrations, exam catalogue, counts and conflicts.
--
-- This is where the per-program collection names go away. `studentregs_IF-B`,
-- `exams_IF-B`, `count_IF-B`, `conflicts_IF-B`, `joint_IF-B` -- roughly five per
-- study program, 100-150 physical collections discovered by running a regex over
-- collection names -- become five tables with a `program` column.
--
-- Everything here is SOURCE DATA: it is replaced wholesale on every Primuss
-- import and can always be re-imported. Per the rule that burned us on exam
-- withdrawal, nothing hand-entered may hang off it by foreign key. That is why
-- exam_primuss_ancode (the ZPA link) and exam_conflict_rating (per-student
-- decisions) deliberately do NOT reference these tables.

-- +goose Up

-- The Primuss exam catalogue. Column names are the German/Primuss ones no more:
-- AnCode/Titel/pruefer/Stg/sonst/ist_praesenz were storage identifiers under
-- MongoDB and become a header mapping inside the importer, where they belong.
create table primuss_exam (
    semester_id text not null references semester(id) on delete cascade,
    program     text not null references study_program(shortname),
    ancode      int  not null,
    module      text not null default '',
    main_examer text not null default '',
    exam_type   text not null default '',
    presence    text not null default '',
    -- Columns the XLSX carries that we do not model. Under MongoDB unknown fields
    -- were kept implicitly; dropping them silently would be a regression.
    raw         jsonb not null default '{}',

    primary key (semester_id, program, ancode)
);

-- Student registrations.
--
-- mtknr is TEXT: Matrikelnummern have leading zeros, and a numeric column would
-- eat them. There is deliberately NO unique constraint on (ancode, mtknr): the
-- Primuss source data really does contain a student registered twice for the same
-- exam, in the current semester and in freshly imported data. A unique index
-- would make the import FAIL rather than protect anything, so the duplicate
-- belongs in the validation report instead (db/indexes.go:33-37 documents the
-- same decision for the Mongo index).
--
-- TWO programs, and they are NOT the same thing. Under MongoDB one of them was
-- the collection name (studentregs_IF) and the other a field inside the document
-- (Stg), so nothing forced them together; a single `program` column here would
-- have quietly merged them.
--
--   program         the EXAM's program -- the former collection suffix, and the
--                   lookup key: "who is registered for (program, ancode)".
--   student_program the STUDENT's own program, Primuss column Stg. This is what
--                   model.StudentReg.Program carries, and it feeds the NTA
--                   program check (plexams/prepare.go:132), model.Student.Program
--                   and the FK07 statistic.
--
-- In 2026-SS they differ for 178 of 10794 registrations (1.6 %) -- exactly the
-- cross-program cases those consumers exist for. student_program deliberately has
-- NO foreign key: 14 of its values (AR, BW, CH, DF, DS, DT, EI, GD, ME, MN, PN,
-- RS, TP, WI) are other faculties' programs and are not in study_program.
create table studentreg (
    id              bigint generated always as identity primary key,
    semester_id     text not null references semester(id) on delete cascade,
    program         text not null references study_program(shortname),
    student_program text not null default '',
    primuss_ancode  int  not null,
    mtknr           text not null,
    group_name      text not null default '',
    name            text not null default '',
    presence        text not null default '',
    -- AASPF is the Primuss field that says which degree a registration belongs
    -- to (84 = Bachelor, 90 = Master). It is the key to telling a DC-B student
    -- from a DC-M one where the study group alone cannot: see aaspf_degree.
    -- INT here, although the XLSX delivers it as the string "84" -- the importer
    -- parses it, exactly like every other numeric Primuss column.
    aaspf           int,
    raw             jsonb not null default '{}'
);

create index studentreg_by_exam_idx on studentreg (semester_id, program, primuss_ancode, mtknr);
create index studentreg_by_student_idx on studentreg (semester_id, mtknr);

-- AASPF code to degree. A lookup table rather than a switch in Go, so a new code
-- is a row someone adds instead of a release.
--
-- The consumer -- resolving an ambiguous ZPA study group like "DC" to DC-B or
-- DC-M in db/zpa_exams.go's degreeSuffixedForGroup hook -- is deliberately NOT
-- built yet: it needs real ZPA group names for dual codes to validate against,
-- and it is a behaviour change that has no business riding along with a storage
-- migration. The column and the mapping exist from the first import so that the
-- data is there when the logic is written, with no re-import needed.
create table aaspf_degree (
    aaspf  int  primary key,
    degree text not null check (degree in ('B', 'M'))
);

insert into aaspf_degree (aaspf, degree) values (84, 'B'), (90, 'M');

-- Primuss' own registration count per exam, an independent source from the
-- registrations themselves. The two drifting apart is a real signal, which is why
-- it stays a separate number rather than a COUNT(*) view: the drift report in
-- plexams/validate_db.go:429 keeps its job.
create table primuss_count (
    semester_id text not null,
    program     text not null,
    ancode      int  not null,
    total       int  not null,
    raw         jsonb not null default '{}',

    primary key (semester_id, program, ancode),
    foreign key (semester_id, program, ancode)
        references primuss_exam(semester_id, program, ancode)
        on delete cascade on update cascade
);

-- THE ancode-keyed document, unrolled.
--
-- In MongoDB each row of conflicts_<PROG> was `{AnCo, Titel, Prüfer, "<ancode>":
-- n, "<ancode>": n, ...}` -- the FIELD NAMES were data. It needed a hand-written
-- decoder (db/primuss_conflicts.go:122-155) that silently filed any key it could
-- not parse under ancode 0, and renaming an ancode meant a $rename across every
-- document in the collection.
--
-- Verified against the live data before choosing the shape: every counterpart
-- ancode lies within the same program (468 of 468 in conflicts_DE), so the
-- composite foreign key below is sound.
--
-- The DIAGONAL is kept on purpose. 37 rows in conflicts_DE carry their own ancode
-- as a key, and its value is exactly the number of registrations for that exam --
-- not a conflict at all. conflictToModelConflicts passes it through today, so
-- there is no `check (ancode <> other_ancode)` here: dropping it is a behaviour
-- change that needs a consumer audit, and storage and behaviour must not change
-- in the same step.
create table primuss_conflict (
    semester_id  text not null,
    program      text not null,
    ancode       int  not null,
    other_ancode int  not null,
    num_students int  not null,

-- ON UPDATE CASCADE on both keys is what retires the $rename. Renumbering a
-- Primuss exam used to be three steps -- $set AnCo in exams, $set AnCo in count,
-- then a $rename of one FIELD NAME across every document of the conflicts
-- collection -- with the data inconsistent between them. Here the single UPDATE
-- of primuss_exam.ancode propagates to the counter, to the conflicts of that exam
-- and to every conflict that names it as a counterpart, atomically.
    primary key (semester_id, program, ancode, other_ancode),
    foreign key (semester_id, program, ancode)
        references primuss_exam(semester_id, program, ancode)
        on delete cascade on update cascade,
    foreign key (semester_id, program, other_ancode)
        references primuss_exam(semester_id, program, ancode)
        on delete cascade on update cascade
);

-- Exams of joint study programs (MUC.DAI, MUC.HEALTH), imported from their CSV.
-- Was joint_<PROG>, keyed by the German column names of the source file.
create table joint_exam (
    semester_id      text not null references semester(id) on delete cascade,
    program          text not null references study_program(shortname),
    primuss_ancode   int  not null,
    module           text not null default '',
    exam_type        text not null default '',
    grading          text not null default '',
    duration_min     int  not null default 0,
    main_examer      text not null default '',
    second_examer    text not null default '',
    is_repeater_exam text not null default '',
    planer           text not null default '',

    primary key (semester_id, program, primuss_ancode)
);

-- The link from a joint exam to the local (external or ZPA) ancode that
-- represents it. status 'unresolved' means the ancode is not known yet, which is
-- a legitimate intermediate state and the reason ancode is nullable.
--
-- The check is what retires the "linked with no ancode" report in
-- plexams/validate_db.go:456.
create table joint_link (
    semester_id    text not null references semester(id) on delete cascade,
    program        text not null references study_program(shortname),
    primuss_ancode int  not null,
    kind           text not null check (kind in ('external', 'zpa')),
    ancode         int,
    status         text not null check (status in ('linked', 'unresolved')),
    source         text not null check (source in ('auto', 'manual')),
    -- Snapshots for display, so the list stays readable when the source is gone.
    module         text not null default '',
    main_examer    text not null default '',

    primary key (semester_id, program, primuss_ancode),
    constraint joint_link_linked_has_ancode
        check ((status = 'linked') = (ancode is not null))
);

-- The prepared per-student view the planning works from: every student with their
-- registrations, their ZPA data and their NTA resolved in one place.
--
-- A computed cache, rebuilt by PrepareStudentRegs from the tables above -- so
-- jsonb, not a normalised copy of data that already exists relationally. Its
-- staleness is tracked per semester in student_regs_state.
create table student_prepared (
    semester_id    text not null references semester(id) on delete cascade,
    mtknr          text not null,
    student        jsonb not null,
    format_version int  not null default 1,

    primary key (semester_id, mtknr)
);

-- Whether student_prepared needs rebuilding. One row per semester.
create table student_regs_state (
    semester_id text primary key references semester(id) on delete cascade,
    dirty       boolean not null default true
);

-- +goose Down

drop table student_regs_state;
drop table student_prepared;
drop table joint_link;
drop table joint_exam;
drop table primuss_conflict;
drop table primuss_count;
drop table aaspf_degree;
drop table studentreg;
drop table primuss_exam;
