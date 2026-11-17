-- Access to the examiner portal.
--
-- A login reaches the planner API through app_user, exactly as before. The portal adds
-- a second way in: a login that IS a ZPA person (matched by e-mail against the teacher
-- mirror) may see and enter its own data. These tables are the manual corrections and
-- extensions to that automatic match, plus the personal access tokens.
--
-- All of them are global -- a login, a delegation or a token does not belong to a
-- semester -- and none has a foreign key to teacher: the teacher mirror is cleared and
-- refilled on every ZPA import (db/zpa_pg.go CacheTeachers), so a foreign key would
-- either block the import or cascade these rows away. ZPA person ids are stable; they
-- are validated in Go when a row is written.
--
-- Every e-mail column is stored lower-cased and trimmed, the form the auth middleware
-- compares; the CHECKs make any other form unrepresentable instead of silently
-- unmatched.

-- +goose Up

-- A login whose SSO address differs from the address ZPA has for the person.
create table login_alias (
    email      text not null check (email <> '' and email = lower(btrim(email))),
    -- 0 means "nobody" in exam.main_examer_id; an alias to it would own every exam
    -- without an examiner.
    teacher_id int  not null check (teacher_id > 0),
    note       text not null default '',
    created_by text not null,
    created_at timestamptz not null default now(),

    primary key (email, teacher_id)
);

-- Withdraws portal access from a login immediately, without touching ZPA. A staff role
-- is unaffected: this is about the portal only.
create table portal_login_block (
    email      text primary key check (email <> '' and email = lower(btrim(email))),
    reason     text not null default '',
    created_by text not null,
    created_at timestamptz not null default now()
);

-- A login that may act in the portal for a ZPA person (Vertretung). Dates rather than a
-- semester: a delegation spans the weeks someone is away, not a planning cycle.
create table portal_delegation (
    delegate_email text not null check (delegate_email <> '' and delegate_email = lower(btrim(delegate_email))),
    teacher_id     int  not null check (teacher_id > 0),
    valid_from     date,
    valid_until    date,
    note           text not null default '',
    created_by     text not null,
    created_at     timestamptz not null default now(),

    primary key (delegate_email, teacher_id),
    check (valid_from is null or valid_until is null or valid_from <= valid_until)
);

-- A planner of another faculty who may see (READ) or later maintain (PLAN) the exams of
-- one study program. Cascades with the program on purpose: losing a grant is safe, a
-- grant blocking the deletion of a program is not.
create table program_grant (
    email       text not null check (email <> '' and email = lower(btrim(email))),
    program     text not null references study_program(shortname) on update cascade on delete cascade,
    level       text not null check (level in ('READ', 'PLAN')),
    valid_until date,
    note        text not null default '',
    created_by  text not null,
    created_at  timestamptz not null default now(),

    primary key (email, program)
);

-- Personal access tokens for the portal API: plexams_<token_id>_<secret>.
create table portal_token (
    -- The public half: 10 bytes of crypto/rand in Crockford base32. The primary key, so
    -- authentication is one indexed lookup -- which is what makes a constant-time
    -- comparison of the secret affordable.
    token_id     text primary key check (token_id ~ '^[0-9A-HJKMNP-TV-Z]{16}$'),
    -- No foreign key: the owner is a login, and a login is not a row anywhere -- most
    -- portal users have no app_user entry. What the token may do is resolved from the
    -- login on every request, so it can never exceed what its owner may do right now.
    owner_email  text not null check (owner_email <> '' and owner_email = lower(btrim(owner_email))),
    -- What the token is for, in the owner's words, so revoking the right one is easy.
    description  text not null default '',
    scope        text not null default 'READ' check (scope in ('READ', 'WRITE')),
    -- SHA-256 of the secret half. Not a password KDF: the secret is 32 bytes from
    -- crypto/rand, there is nothing to brute-force, and a KDF would cost tens of
    -- milliseconds on every API call. This rests on the server always generating the
    -- secret; there is no bring-your-own-token path.
    secret_hash  bytea not null check (length(secret_hash) = 32),
    created_at   timestamptz not null default now(),
    -- There is no token that never expires. Default and ceiling are enforced where tokens
    -- are minted, because a CHECK against now() is not immutable.
    expires_at   timestamptz not null,
    -- Written at most every few minutes: a write per call would serialise a script.
    last_used_at timestamptz,
    -- Revocation is a timestamp, not a DELETE: the audit log keeps resolving the id.
    revoked_at   timestamptz,

    check (expires_at > created_at)
);
create index portal_token_owner_idx on portal_token (owner_email);

-- The portal resolves a login against the teacher mirror of the newest semesters, not
-- only the active one. The existing index leads with semester_id.
create index teacher_login_email_idx on teacher (lower(btrim(email)));

-- +goose Down

drop index teacher_login_email_idx;
drop table portal_token;
drop table program_grant;
drop table portal_delegation;
drop table portal_login_block;
drop table login_alias;
