-- The ~33 generator weights are always read and written as a whole and never
-- queried by field, so they live in one jsonb column instead of 33 columns that
-- would need a migration every time a weight is added.
--
-- format_version travels with the blob: the json tags are the GraphQL contract
-- too, so renaming a field in the .graphqls would silently change the storage
-- format. Reading a version this binary does not know has to fail loudly.
-- name: GetGenerationConfig :one
select * from generation_config where id = 1;

-- name: SetGenerationConfig :exec
insert into generation_config (id, config, format_version)
values (1, $1, $2)
on conflict (id) do update set
    config         = excluded.config,
    format_version = excluded.format_version;
