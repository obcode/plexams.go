---
name: deploy-push-cd
description: "Deploy automation for plexams.cs.hm.edu — ghcr images + self-hosted runner push-CD; open: notification + Mongo backup."
metadata:
  node_type: memory
  type: project
---

**BUILT 2026-07-14 (branch `feat/deploy-push-cd` → merged to main).** plexams.go +
plexams.gui are deployed on `plexams.cs.hm.edu` as user **`plexams` under `/home/plexams`**
(deploy dir `/home/plexams/deploy`), VPN-only reachable. See [[auth-roles-shibboleth]] for the
nginx + oauth2-proxy OIDC stack this builds on.

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

**Operator TODO (outside repos):** register the runner on BOTH repos (label `plexams-deploy`,
run as user plexams, in docker group), set `AUTO_DEPLOY=true` (+ optional `DEPLOY_DIR`) in each.

**Open (Oliver asked 2026-07-14, proposals pending, NOT built):**
1. Deploy notification — leaning "footer version is enough"; cheapest = GitHub *watch releases*
   email, no code. Optional job-summary line.
2. **MongoDB backup strategy** — idea was periodic `mongodump` pushed to a gitlab.lrz.de repo.
   Caveat: student PII ⇒ git-forever + plaintext is bad; recommend host-cron `mongodump
   --archive --gzip` + gpg/age encrypt + offsite (rotation), git only as encrypted offsite store.
