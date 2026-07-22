---
name: admin-overview-digest
description: "ADMIN platform-overview GraphQL query + daily digest mail to all ADMIN users; backend DONE & merged to main, GUI page pending."
metadata:
  node_type: memory
  type: project
  originSessionId: 9ee18026-734c-4ae1-8d4f-14dac1922c1d
  modified: 2026-07-22T15:56:55.055Z
---

Admin-Betriebsüberblick + täglicher Digest (Wunsch von Oliver, einziger ADMIN). Rein additiv, komponiert vorhandene Quellen.

**Backend DONE & auf main gemergt** (semilinear, feature-Branch gelöscht; build/vet/golangci-lint-v2/tests grün):
- `adminOverview: AdminOverview!` + `schedulerStatus: SchedulerStatus!` Queries, beide mit `requireAdmin(ctx)` IM RESOLVER (Pitfall: `AroundOperations` sperrt VIEWER nur aus Mutationen, nicht aus Queries). Schema `graph/admin_overview.graphqls`, Logik `plexams/admin_overview.go` (`AdminOverview`, `SchedulerStatus`, reine `computeActivitySummary`/`roleCounts`/`topOperations`).
- Zwei bisher nicht exponierte Quellen freigelegt: `scheduler_state` (letzter Auto-Sync-Lauf) via `SchedulerStatus`, und `opGuard` (`WritesAllowed`) + `IsReadOnly` via `LiveStatus`.
- Scope: Aktivität/Audit/Sync fürs AKTIVE Semester; Users/Scheduler/Workspaces global. Workspaces = wiederverwendetes `Semester`-Model (keine neue Type).
- **Täglicher Digest** `plexams/admin_overview_mail.go` `SendAdminDigest(ctx, run, reporter)`: Template `plexams/email/tmpl/adminDigest.md.tmpl` (+ Katalog-Eintrag in `email_template_catalog.go` — PFLICHT, sonst brechen Template-Tests) → `sendSystemMail`. Empfänger selbstpflegend via `adminRecipientsFromUsers` (alle RoleAdmin, Fallback `scheduler.adminmail.recipient`→`scheduler.debugrecipient`; leer+run⇒skip).
- Zweite Scheduler-Instanz `startAdminDigestMail` (`graph/scheduler.go`, Keys `scheduler.adminmail.enabled`/`.time` Default 06:00, kein CatchUp/kein State-Write → clobbert Auto-Sync-Anker nicht), in `graph/server.go` gestartet+Shutdown neben `sched`.
- On-demand `sendAdminDigestNow(dryRun): LogLine!` Subscription (spiegelt `triggerScheduledSync`, requireAdmin, dryRun→Testmail).
- Config-Doku in `deploy/.plexams.yaml.example`. Golden-Test `TestAdminDigestGolden` + Unit-Tests in `plexams/admin_overview_test.go`.

**GUI pending** (plexams.gui, separates Repo): ADMIN-only-Seite die `adminOverview` konsumiert (Karten: Zugriff/Rollen, Auto-Sync-Status, Aktivität+Audit-Ausschnitt mit Link auf `mutationLog`-Filter, Backup, Workspaces, Server-Version); Menüpunkt nur wenn `myAccount.role==ADMIN`; Button „Digest jetzt senden (Testmail)" → `sendAdminDigestNow(dryRun:true)`.

Verwandt: [[nightly-autosync-zpa-anny]] (Scheduler/Mail-Muster), [[auth-roles-shibboleth]] (ADMIN-Rolle), [[emails-over-graphql]], [[email-markdown-templates]].
