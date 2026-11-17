---
name: portal-access
description: Examiner portal backend (M1) — Principal with staff and portal sides, /query/portal and /query/token, PATs, login resolution, root field policy, admin access API
metadata:
  type: project
---

Built 2026-11 as M1 of the examiner portal. Everything is behind `portal.enabled`
(default false: neither route is mounted, a login without app_user gets 403 as before).

- **Principal** (`principal/principal.go`): `Staff *model.User` plus portal side
  (`TeacherIDs`, `ActsFor`, `Grants`, `Blocked`, `Token`). `UserFromContext` returns
  **only Staff** — the planner API treats any non-nil user as allowed to read everything,
  so a portal-only login must look anonymous there. Never widen that.
- **Login resolution** (`ResolvePortalLogin`, one SQL union in
  `db/queries/portal_access.sql`): teacher rows of the 3 newest teacher snapshots with that
  e-mail (lower/trim, `id > 0` — 0 means "nobody") plus `login_alias`; `portal_login_block`
  withdraws portal access only. Not `GetTeacherByEmail`: that follows the active semester.
- **Two doors, one separate gqlgen server** (`gqlgen.portal.yml`, `graph/portal/`,
  `graph/portal_server.go`): `/query/portal` (SSO header) and `/query/token` (Bearer PAT
  only, no header fallback; `portal.tokens`). POST only, complexity limit, introspection on,
  `extensions.code` on errors, rate limit per token. The resolver knows only
  `plexams/portal.Service`, never `*Plexams`.
- **PAT** (`plexams/pat`): `plexams_<16 Crockford>_<43 base64url>`, SHA-256 stored,
  constant-time compare incl. dummy hash, 90 d default / 365 max, READ default. Minting and
  listing only in a session (INTERACTIVE_ONLY); READ tokens cannot mutate. Rule in
  `.gitleaks.toml`.
- **Students:** first barrier is the ZPA match; `portal.denystudents` + `auth.affiliationheader`
  is the second, pending the oauth2-proxy claim switch.
- **Root field policy** (`graph/root_field_policy.txt`, `graph/policy.go`): every planner
  root field needs an entry, enforced in AroundFields; a new field without one fails
  `TestRootFieldPolicyCoversTheSchema` (regenerate with `UPDATE_ROOT_FIELD_POLICY=1`).
  Account/access mutations also go into the read-only exemptions (`graph/readonly.go`).
- **Admin API** (`graph/access.graphqls`, ADMIN): aliases, blocks, tokens,
  `accessIdentityReport`, `accessResolveLogin`, `rejectedLogins` (in-memory ring of 50).

Next (M2): inquiries/submissions in a semester-explicit `plexams/inquiry` package, change
notification mails, `server.publicurl`.
