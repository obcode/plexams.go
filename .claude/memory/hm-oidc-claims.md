---
name: hm-oidc-claims
description: "HM sso.hm.edu OIDC scope→claim mapping; minimal id_token, claims from UserInfo; department=fhmDepartment."
metadata:
  node_type: memory
  type: reference
  originSessionId: 86380ad4-08bb-4df6-a6df-32e4698f5b08
---

sso.hm.edu (Shibboleth OIDC, issuer `https://sso.hm.edu`) scope → released claims,
from `.well-known/openid-configuration` (confirmed 2026-07-13 during Shibboleth smoketest):

- **openid**: `sub` (subject-public or subject-pairwise)
- **email**: `email`, `email_verified`
- **profile**: `name`, `family_name`, `given_name`, `preferred_username`, `email`, `eduPersonPrincipalName`, `eduPersonScopedAffiliation`
- **phone**: `phone_number` (from telephoneNumber)
- **department**: `fhmDepartment`  ← note the claim name, NOT `department`. CONFIRMED
  released to client `plexams.cs.hm.edu` even though `department` is NOT in the discovery
  `scopes_supported` (it's a client-specific custom scope). Value is the faculty number,
  e.g. `"07"` = FK07. A scope change requires a FRESH login (logout + re-auth) to mint a
  new token — reloading reuses the old token/scope and the claim stays empty.
- **mifare**: `hmMifareSerial`

Key behavior: the **id_token is minimal** — only `sub`, `eduPersonPrincipalName`,
and standard fields (at_hash/aud/acr/auth_time/iss/exp/iat/sid). email,
preferred_username, fhmDepartment, etc. are NOT in the id_token; they come from
the **UserInfo endpoint**, which oauth2-proxy's claim extractor also queries.
`end_session_endpoint` = `https://sso.hm.edu/idp/profile/Logout` (RP-initiated /
central SO — we deliberately do NOT use it; plexams uses local logout only, see
`/abgemeldet` landing). `login endpoint`: `/idp/profile/oidc/authorize`.
MFA acr released: `https://refeds.org/profile/mfa`.

To surface an arbitrary claim (e.g. fhmDepartment) through oauth2-proxy you need
`--alpha-config` injectResponseHeaders (the simple env config has no per-claim
custom header). In the smoketest we cheat via `OAUTH2_PROXY_OIDC_GROUPS_CLAIM=fhmDepartment`
to display it in the groups slot. The plexams backend authorizes on **email**
only (X-Remote-User + DB allow-list), so department is not required in production.
See [[auth-roles-shibboleth]]; `eduPersonScopedAffiliation` is the natural signal
if role/affiliation gating is ever wanted.
