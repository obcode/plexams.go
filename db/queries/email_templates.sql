-- name: ListEmailTemplates :many
select * from email_template order by name collate "C";

-- name: GetEmailTemplate :one
select * from email_template where name = $1;

-- name: SetEmailTemplate :exec
insert into email_template (name, markdown)
values ($1, $2)
on conflict (name) do update set markdown = excluded.markdown;

-- name: DeleteEmailTemplate :execrows
delete from email_template where name = $1;
