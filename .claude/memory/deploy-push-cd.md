---
name: deploy-push-cd
description: Deploy automation for plexams.cs.hm.edu — ghcr images + self-hosted runner push-CD; LIVE & auto-deploying for BOTH plexams.go and plexams.gui.
metadata:
  node_type: memory
  type: project
  originSessionId: ce64cc7e-14d0-4b61-8721-724a749f3a4c
---

**BUILT 2026-07-14 (branch `feat/deploy-push-cd` → merged to main).** plexams.go +
plexams.gui are deployed on `plexams.cs.hm.edu` as user **`plexams` under `/home/plexams`**
(deploy dir `/home/plexams/deploy`), VPN-only reachable. See [[auth-roles-shibboleth]] for the
nginx + oauth2-proxy OIDC stack this builds on.

**UPDATE 2026-07-15 — the reverse proxy is now Caddy, not nginx** (see [[deploy-caddy-migration]]):
the `location /` → gui:3000 reverse-proxy + auth is now a Caddy `reverse_proxy`/`forward_auth`, and
the CI deploy job syncs the `Caddyfile` (not nginx templates). Everything else below still holds.

Automation = **pull-of-prebuilt-images + push-CD via a self-hosted runner ON the deploy host**
(VPN-only ⇒ GitHub can't push in; the runner polls outbound). What changed in `deploy/`:
- `docker-compose.yml`: `plexams` now `image: ghcr.io/obcode/plexams.go:${PLEXAMS_TAG:-latest}`
  (no local build); new `gui` service `ghcr.io/obcode/plexams.gui:${GUI_TAG:-latest}` exposing
  3000; nginx reverse-proxies `location /` to `gui:3000` (auth_request stays at the proxy, no
  X-Remote-* to the GUI); `gui-dist` volume dropped.
- `.env`: `PLEXAMS_TAG` / `GUI_TAG` (CI pins them to the released version; rollback = edit + `up -d`).
- `.github/workflows/docker.yml`: new `deploy` job, `needs: build-and-push-image`,
  `runs-on: [self-hosted, plexams-deploy]`, **gated by repo variable `AUTO_DEPLOY=true`** (so it
  stays skipped until the runner exists — no stuck queue). Syncs non-secret infra (compose +
  nginx templates) into `${DEPLOY_DIR:-/home/plexams/deploy}`, pins tag in `.env`, `compose pull
  && up -d`. Secrets (`.env`/`.plexams.yaml`/`tls/`) live on-host and are never touched.
- plexams.gui repo: mirror Dockerfile (adapter-node, non-root, 3000), docker.yml build+push +
  deploy job (pins `GUI_TAG`, `up -d gui`), semantic-release — all confirmed done by Oliver.

**LIVE 2026-07-15 (Oliver confirmed):** auto-deploy is fully running end-to-end for BOTH
repos — a release of plexams.go OR plexams.gui builds+pushes the ghcr image and the self-hosted
runner on the host pulls & `up -d`s it. Runners are containerized (Alpine host, `git log`
`c69ced9`); a **second self-hosted runner** was added for the plexams.gui repo (`3e1feb8`).
Operator setup (register runner on both repos, `AUTO_DEPLOY=true`) = DONE.

**Deploy notification — DROPPED (Oliver 2026-07-15):** not building it; the GitHub *watch
releases* email is enough. Footer already shows the version. Nothing to do here.

**MongoDB backup — BUILT 2026-07-14 (on main):** Oliver chose local rotation + a GUI planner
ZIP prompt (NOT the gitlab.lrz.de push).
- Host: `deploy/backup/mongo-backup.sh` — `docker compose exec mongo mongodump --archive --gzip`
  of ALL dbs → `/home/plexams/backups`, rotation 14 daily + 8 weekly; empty-dump guard; restore
  cmd in header. Schedule via busybox `crond` (README). No offsite (add scp/rclone to extend).
- Backend: `SemesterMeta.LastDumpAt` stamped in `HTTPDownloadSemesterDump`; `db.LatestMutationTime`;
  new GraphQL `backupStatus { hasUnsavedChanges, lastDumpAt, lastChangeAt }` (hasUnsavedChanges =
  lastChangeAt after lastDumpAt, or never dumped). See [[semester-dump-restore]] for the ZIP.
- **GUI pending (Teil 3):** poll `backupStatus`, show a subtle prominent banner/button linking to
  `/download/semester-dump.zip` when hasUnsavedChanges; download stamps lastDumpAt → banner clears.
