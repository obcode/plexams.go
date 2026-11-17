-- Portal access: resolving a login, aliases, blocks, delegations, grants, tokens.

-- Everything the portal needs to know about a login, in one round trip. One row per
-- fact; kind says which. The staff role is NOT part of this: it keeps coming from
-- app_user through GetUser, so the planner path is untouched by the portal.
--
-- Teachers are matched in the newest @snapshots semesters that have a teacher mirror,
-- not only in the active one: identity must not follow the semester switch. Not in all
-- semesters either: old mirrors keep superseded addresses forever. Semester ids sort
-- correctly as text ("2026-SS" < "2026-WS" < "2027-SS"). id > 0 because 0 is "nobody".
-- name: ResolvePortalLogin :many
select 'block'::text as kind, 0::int as teacher_id, ''::text as code, ''::text as label
from portal_login_block b where b.email = @email::text
union all
select 'teacher', t.id, '', t.fullname
from (
    select distinct on (te.id) te.id, te.fullname
    from teacher te
    where te.id > 0
      and lower(btrim(te.email)) = @email::text
      and te.semester_id in (
          select s.semester_id from (select distinct semester_id from teacher) s
          order by s.semester_id desc limit @snapshots::int)
    order by te.id, te.semester_id desc
) t
union all
select 'alias', a.teacher_id, '', ''
from login_alias a where a.email = @email::text
union all
select 'delegation', d.teacher_id, '', ''
from portal_delegation d
where d.delegate_email = @email::text
  and coalesce(d.valid_from, current_date) <= current_date
  and coalesce(d.valid_until, current_date) >= current_date
union all
select 'grant', 0, g.level, g.program
from program_grant g
where g.email = @email::text
  and coalesce(g.valid_until, current_date) >= current_date;

-- name: ListLoginAliases :many
select * from login_alias order by email collate "C", teacher_id;

-- name: InsertLoginAlias :exec
insert into login_alias (email, teacher_id, note, created_by)
values (@email, @teacher_id, @note, @created_by)
on conflict (email, teacher_id) do update set note = excluded.note;

-- name: DeleteLoginAlias :execrows
delete from login_alias where email = @email and teacher_id = @teacher_id;

-- name: ListPortalLoginBlocks :many
select * from portal_login_block order by email collate "C";

-- name: InsertPortalLoginBlock :exec
insert into portal_login_block (email, reason, created_by)
values (@email, @reason, @created_by)
on conflict (email) do update set reason = excluded.reason;

-- name: DeletePortalLoginBlock :execrows
delete from portal_login_block where email = @email;

-- name: InsertPortalToken :exec
insert into portal_token (token_id, owner_email, description, scope, secret_hash, expires_at)
values (@token_id, @owner_email, @description, @scope, @secret_hash, @expires_at);

-- name: GetPortalToken :one
select * from portal_token where token_id = @token_id;

-- name: ListPortalTokensOf :many
select * from portal_token where owner_email = @owner_email order by created_at desc;

-- name: ListAllPortalTokens :many
select * from portal_token order by owner_email collate "C", created_at desc;

-- Revoking is idempotent in effect but reports whether anything was still active, so
-- the caller can tell "revoked" from "not yours / unknown / already revoked".
-- name: RevokePortalTokenOf :execrows
update portal_token set revoked_at = now()
where token_id = @token_id and owner_email = @owner_email and revoked_at is null;

-- name: RevokePortalToken :execrows
update portal_token set revoked_at = now()
where token_id = @token_id and revoked_at is null;

-- At most one write per token every five minutes: a script's burst of calls costs one
-- row update, not one per request.
-- name: MarkPortalTokenUsed :exec
update portal_token set last_used_at = now()
where token_id = @token_id
  and (last_used_at is null or last_used_at < now() - interval '5 minutes');

-- The newest mirror row of each requested ZPA person, independent of the active
-- semester: names for the portal's session view.
-- name: PortalTeachers :many
select distinct on (id) id, shortname, fullname
from teacher
where id = any(@ids::int[])
order by id, semester_id desc;
