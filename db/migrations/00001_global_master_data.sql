-- Global master data: semester-independent entities that carry over between
-- planning cycles. In MongoDB these lived in the separate `plexams` database;
-- here they are simply the tables without a semester_id column.

-- +goose Up

-- Nachteilsausgleich. Keyed by Matrikelnummer, which is TEXT and never numeric:
-- the numbers have leading zeros.
--
-- valid_from / valid_until are TEXT on purpose. They are free-form semester
-- labels in the model (model.NTA.From/Until), not dates -- do not "improve"
-- them into date columns without changing the model first.
create table nta (
    mtknr                  text    primary key,
    name                   text    not null,
    email                  text,
    compensation           text    not null,
    delta_duration_percent int     not null,
    needs_room_alone       boolean not null default false,
    needs_hardware         boolean not null default false,
    program                text    not null,
    valid_from             text    not null,
    valid_until            text    not null,
    last_semester          text,
    deactivated            boolean not null default false
);

-- +goose Down

drop table nta;
