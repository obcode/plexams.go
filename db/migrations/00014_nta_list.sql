-- The NTA lists the exam board publishes per faculty in a SharePoint folder (a
-- PRIMUSS export, one xlsx per faculty and semester). The last uploaded list per
-- faculty is kept as it was read: the planners compare it with the NTA master data
-- (table nta, which is only changed by an explicit apply step) and its
-- registrations with the Primuss registrations, which are often more recent.
--
-- fk is the faculty number with two digits ("07", "21" = MUC.DAI). A new upload of
-- the same faculty replaces the previous list.

-- +goose Up

create table nta_list (
    semester_id   text        not null references semester(id) on delete cascade,
    fk            text        not null,
    filename      text        not null,
    -- the PRIMUSS semester code of the list ("226") and its plexams id; the list
    -- may belong to another semester than the one it was uploaded into.
    semester_code text        not null default '',
    list_semester text        not null default '',
    stand         timestamptz,
    warnings      jsonb       not null default '[]',
    imported_at   timestamptz not null default now(),
    imported_by   text,

    primary key (semester_id, fk)
);

-- One student of a list. row is the parsed row (package ntalist); first_seen is
-- the time of the first upload of this faculty's list that contained the mtknr,
-- carried over from list to list.
create table nta_list_entry (
    semester_id text        not null,
    fk          text        not null,
    line        int         not null,
    mtknr       text        not null,
    row         jsonb       not null,
    first_seen  timestamptz not null default now(),

    primary key (semester_id, fk, line),
    foreign key (semester_id, fk) references nta_list(semester_id, fk) on delete cascade
);

create index nta_list_entry_mtknr_idx on nta_list_entry (semester_id, mtknr);

-- +goose Down

drop table nta_list_entry;
drop table nta_list;
