---
name: jira-integration
description: "On-prem Jira (jira.cc.hm.edu) integration via PAT — backend done on feat/jira-integration, GUI still pending"
metadata:
  node_type: memory
  type: project
  originSessionId: 9f8ded3a-d083-4efa-aad0-c5ed29e9828b
---

On-prem Jira (jira.cc.hm.edu, Data Center) is integrated into the backend via a
Personal Access Token (`Authorization: Bearer <PAT>`). New `jira/` package mirrors
`zpa/`. Committed on branch `feat/jira-integration` (commit e348b3e, 2026-07-08).

**Config** (`.plexams.yaml`, secret stays in file, never in DB):
`jira.baseurl`, `jira.token` (PAT), `jira.project` (default project key —
**FK07PP** at HM; issue keys look like FK07PP-123).

**Surface built:**
- GraphQL queries `jiraConnection`/`jiraIssue`/`jiraTransitions`/`jiraOpenIssues`/
  `jiraOpenIssuesByType`/`jiraOpenIssuesByRequestType`; mutations `createJiraIssue`/
  `addJiraComment`/`transitionJiraIssue`. Open-issue listing via JQL search
  (`statusCategory != Done`).
- **FK07PP is a JSM (service desk) project**: "Anfragetyp" = customer request type,
  a customfield (NOT issue type). Its id is discovered at runtime via
  /rest/api/2/field (schema.custom == com.atlassian.servicedesk:vp-origin) and
  cached; see jira/servicedesk.go.
- REST `POST /upload/jira-attachment` (multipart: key, file) for binary
  attachments (PDFs/CSVs) — binary goes through REST like the other up/downloads,
  not GraphQL. See [[emails-over-graphql]] for the same split.
- `Plexams.SetJira()` (lazy, no network) / `TestJira()` (calls /myself).

**Decisions:** usage is manual/GUI-driven only (no auto-issue-creation). project
and issueType are intentionally flexible per call (fall back to config default /
"Task") because the user hadn't decided the project/issue-type mapping yet.

`JiraIssue` also carries `reporter` (author, in list+detail) and `comments`
(author/body/created; only populated by `jiraIssue(key)`, which fetches the full
thread). Verified live against FK07PP 2026-07-08: connects as hm-obraun, 16 open
issues, all request type "EXaHM / SEB", reporter+comments parse correctly.
Config lives in `/home/ubuntu/.plexams.yml` (note `.yml`), portal 13.

**Still pending:** the plexams.gui side (forms/buttons to create issues, comment,
transition, upload attachments, list/group views). No TLS tweak was needed
(plain HTTPS works). Branch feat/jira-integration not yet pushed/PR'd.
