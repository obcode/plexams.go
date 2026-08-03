-- The planer table is a singleton (id = 1 enforced by a check), the successor of
-- the single document the Mongo version found with an empty filter.
--
-- model.Planer's DefaultMail and the four Effective* fields are derived at read
-- time from the overrides plus config, and are deliberately not columns here.
-- Mongo stored them along with everything else, which is how a stale derived
-- value could outlive the override it came from.
-- name: GetPlaner :one
select * from planer where id = 1;

-- name: SavePlaner :exec
insert into planer (id, name, email, test_mail, cc, noreply_mail, noreply_name)
values (1, $1, $2, $3, $4, $5, $6)
on conflict (id) do update set
    name         = excluded.name,
    email        = excluded.email,
    test_mail    = excluded.test_mail,
    cc           = excluded.cc,
    noreply_mail = excluded.noreply_mail,
    noreply_name = excluded.noreply_name;
