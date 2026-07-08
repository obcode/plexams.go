---
name: cli-to-gui-migration
description: "Plan/direction to migrate plexams.go CLI fully to the GraphQL + Svelte GUI (plexams.gui), config into DB"
metadata:
  node_type: memory
  type: project
  originSessionId: 6285039b-3933-4bb1-a8f3-24a7355c4a1d
---

Oliver wants to move all `cmd/` CLI functionality into the Svelte web frontend (plexams.gui) via the existing GraphQL API, and move per-semester config out of `.plexams.yaml`/`<semester>.yaml` into MongoDB, editable through the GUI.

Decisions made (2026-06-17):
- **Stays a local single-user tool** — never hosted, never multi-user. → No authentication needed; localhost-only CORS stays.
- **Secrets stay in a config file** (bootstrap): `db.uri`/`db.database`, and SMTP/ZPA/anny credentials. Everything else (per-semester `semesterConfig.*`, `goslots`, `roomconstraints`, `duration`, `donotpublish`, `publish.additionalExams`, `knownConflicts`, `specialInterests`, `invigilation.optimizer.*`) moves to DB + GUI forms.
- Don't switch technology — GraphQL is the right fit; resolvers are ~90% thin passthroughs to `*plexams.Plexams` methods.

Key technical facts:
- Architecture is well-suited: CLI and GraphQL are both thin facades over `Plexams` methods.
- Current GraphQL write coverage vs CLI is only ~45-50%; `addExamToSlot`/`rmExamFromSlot` are stubbed with `panic`.
- `NewPlexams` (plexams/plexams.go) builds the struct ONCE from viper at startup and computes `semesterConfig`/`GoSlots` once (`setSemesterConfig`/`setGoSlots`, incl. GoDay0 offset). DB-editable config needs a `Reload()` that recomputes these.
- `db.SaveSemesterConfig` (db/database.go:40) already writes the *computed* config snapshot to DB; migration inverts this so DB is the source.
- Four cross-cutting concerns the CLI does for free: interactive confirms (trivial in GUI), file output (37 cmds → REST download endpoints, not GraphQL), long-running jobs (ZPA import, invigilation generate/simulated annealing → job pattern), and (dropped) auth.

SAFETY CONSTRAINT (added 2026-06-17): A real planning phase is in progress (room + invigilation
planning unfinished; entries still being added to semesterConfig/roomconstraints in YAML). Therefore:
- Order REVERSED from original: do additive Phase 2 (write-API) and Phase 3 (exports) FIRST — they
  only add GraphQL mutations / REST endpoints, don't change YAML workflow or DB write behavior.
  Phase 1 (config-to-DB) only AFTER current room/invig planning finishes, or with YAML-fallback.
- New config source goes in a NEW collection `raw_config`, NOT the existing `semester_config`
  (which is already dropped+rewritten every startup — disposable snapshot).
- Always mongodump before DB-touching work; develop against a copy (restore as 2026-SS-test).

Full plan persisted in the repo: docs/cli-to-gui-migration-plan.md (git-tracked, the durable source
of truth — resume from there).

PROGRESS (2026-06-25, session 6285039b): per-semester config → DB is **done** for the
`semesterConfig.*`/`goslots` block (3 commits on `main`: cad96ef, dc055bc, ba8d231):
- raw editable `model.SemesterConfigInput` in collection `semester_config_input` (the
  source of truth; the old `semester_config` snapshot is still written for the GUI to read).
- `setSemesterConfig`/`setGoSlots` were split into `semesterConfigInputFromViper` (read YAML) +
  pure `deriveSemesterConfig`/`deriveGoSlots`; boot is now `loadSemesterConfig` (DB → YAML
  fallback + auto-migrate once). `initConfig` no longer requires `<semester>.yaml` (optional).
- GraphQL: `semesterConfigInput`, `setSemesterConfigInput(input)` (warns on plan-invalidating
  changes), `newSemesterConfigDefaults`, `createSemester(semester,input)` (cross-DB, refuses
  overwrite). CLI `init` now writes to the DB, not YAML.
STILL ON YAML (next candidates, same pattern): `rooms.timelag`, `roomconstraints.additionalseats`,
`invigilation.optimizer.*`, `donotpublish`, `publish.additionalExams`, `knownConflicts`,
`specialInterests`, `mucdaiprograms`, `coverPages.*`, `invigilationStats.*`, `duration`.
Decided workflow change: now committing this work directly on `main` (user opted in 2026-06-25);
feature branches not required for these increments.

CLI SHUTDOWN COMPLETE (2026-07-08): the Cobra `cmd/` package and the separate `zpa/cli`
debug main were **deleted**; `plexams.go` is now a server-only GraphQL/REST binary. New
`bootstrap/` package (`bootstrap.Serve()`) holds what was in `cmd/root.go` — config load +
`newPlexams` (was `initPlexamsConfig`) + `graph.StartServer`; only flags left are
`-v`/`--db-uri`/`--semester`, parsed with stdlib `flag` (cobra dropped from go.mod). Last
CLI-only gaps filled: Phase-3 file exports are now REST downloads on the chi router —
`GET /download/pdf/{kind}` (handler `HTTPDownloadPDF` + `pdfExports()` registry; `draft-si`
returns a ZIP), `GET /download/csv/{kind}` (`HTTPDownloadCSVDraft`, `draft?program=`),
`GET /download/ics/{program}` (`HTTPDownloadICS`); generators refactored to share a `*Maroto`
builder / `*Bytes` producer, no more file writers (all `Export*`/`Draft*PDF`/`CsvFor*` file
helpers removed). New mutations `addStudentReg`/`removeStudentReg` (primuss.graphqls). Dropped
without replacement: all `info …` diagnostics, `invigilation problem`, `ics import-mucdai`
(`ReadMucdaiICS`). This resolves the old CLI-fate divergence — CLI does NOT survive. See
[[gui-and-cli-sync]] (now GUI-only) and [[cli-to-gui-migration]].

Also (2026-07-08): the per-semester `<semester>.yaml` merge is **fully removed** from the
bootstrap (`loadPerSemesterYAML` + `semester-path` + the fsnotify watch are gone). Only the
single global `.plexams.yaml` is read now; all per-semester config is in the DB
(`semester_config_input`, GUI-edited). Config split is authoritatively documented in
docs/configuration.md (§6 base-YAML operational keys, §7 moved-to-DB, §8 removed).
