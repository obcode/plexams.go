-- name: GetUserSecret :one
select * from user_secret where email = $1;

-- The four Jira columns are written together: the schema check enforces that a
-- sealed value is all-or-nothing, because a half-written secret cannot be opened.
-- name: SaveUserJiraToken :exec
insert into user_secret (email, jira_key_version, jira_nonce, jira_ciphertext, jira_updated_at)
values ($1, $2, $3, $4, $5)
on conflict (email) do update set
    jira_key_version = excluded.jira_key_version,
    jira_nonce       = excluded.jira_nonce,
    jira_ciphertext  = excluded.jira_ciphertext,
    jira_updated_at  = excluded.jira_updated_at;

-- Clears only the Jira secret and keeps the row, mirroring the Mongo $unset. A
-- user without a stored secret is left alone rather than created empty.
-- name: DeleteUserJiraToken :exec
update user_secret set
    jira_key_version = null,
    jira_nonce       = null,
    jira_ciphertext  = null,
    jira_updated_at  = null
where email = $1;
