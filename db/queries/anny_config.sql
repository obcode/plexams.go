-- name: GetAnnyConfig :one
select * from anny_config where id = 1;

-- name: SetAnnyConfig :exec
insert into anny_config (id, personalization_names)
values (1, $1)
on conflict (id) do update set personalization_names = excluded.personalization_names;
