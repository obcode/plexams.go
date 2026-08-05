-- The planner moves from a global singleton to the semester.
--
-- `planer` was one row for the whole installation, so the planner shown in every
-- mail and on every draft PDF was whoever held the job *now* -- re-sending a mail
-- for a finished semester relabelled it. There are two tiers now: the server
-- config (planer.* in .plexams.yaml) is the default, and this table overrides it
-- per semester. The middle tier -- a globally stored, GUI-editable planner -- is
-- gone: with one deployment it had no job the config did not already do, and
-- three places to look is two too many when a From address turns out wrong.

-- +goose Up

create table semester_planer (
    semester_id  text primary key references semester(id) on delete cascade,
    -- name and email are ONE identity: overriding only the address would send as
    -- "Oliver Braun <someone.else@hm.edu>". The four sender overrides below do
    -- merge field by field -- they are independent settings, not an identity.
    name         text,
    email        text,
    test_mail    text,
    cc           text,
    noreply_mail text,
    noreply_name text,

    constraint semester_planer_identity check (num_nonnulls(name, email) in (0, 2))
);

-- Freeze the current global planner onto every registered semester. Without this
-- the semesters planned before the split would silently start inheriting from the
-- config, which is the one thing this migration must not change.
insert into semester_planer (semester_id, name, email, test_mail, cc, noreply_mail, noreply_name)
select s.id, p.name, p.email, p.test_mail, p.cc, p.noreply_mail, p.noreply_name
from semester s cross join planer p;

drop table planer;

-- +goose Down

create table planer (
    id           int  primary key check (id = 1),
    name         text not null,
    email        text not null,
    test_mail    text,
    cc           text,
    noreply_mail text,
    noreply_name text
);

-- Restore the global row from the newest semester that has a planner of its own.
-- Any per-semester planner that differed from it cannot be represented here and
-- is lost -- that is what rolling back this split means.
insert into planer (id, name, email, test_mail, cc, noreply_mail, noreply_name)
select 1, sp.name, sp.email, sp.test_mail, sp.cc, sp.noreply_mail, sp.noreply_name
from semester_planer sp
where sp.name is not null
order by sp.semester_id desc
limit 1;

drop table semester_planer;
