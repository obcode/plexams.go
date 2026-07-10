---
name: auth-roles-shibboleth
description: "Planned deployment + role concept — Shibboleth (Apache mod_shib) auth, DB users collection, audit \"who did what\"; plan file persisted, build later."
metadata:
  node_type: memory
  type: project
  originSessionId: 96f9d929-9c15-41cd-b151-07bb987f0f76
---

Plan persistiert 2026-07-08 in `~/.claude/plans/ich-will-diese-anwendung-cached-shamir.md` (Start-Prompt am Ende der Datei).

**UPDATE 2026-07-10 — Phase 0 + Deploy-Artefakte GEBAUT auf Branch `feat/oidc-auth` (noch nicht committet).** Entscheidung geklärt: **OIDC statt SAML** — `sso.hm.edu` ist Shibboleth-IdP mit OIDC-OP (`/idp/profile/oidc/...`, issuer `https://sso.hm.edu`, RemoteUser-Claim = `email`, PKCE nicht annonciert → Code-Flow mit client_secret). Backend-Design ist mechanismus-agnostisch (vertraut nur `X-Remote-User`-Header), daher Plan 1:1 gültig, nur Proxy = **Apache `mod_auth_openidc`** statt `mod_shib`. Umgesetzt: `db/users.go` (+`collectionUsers`), `plexams/users.go` (Get/Set/Remove/GetByEmail/SeedUsers aus `auth.seedusers`/LocalDevUser), `graph/auth.graphqls` (`enum Role{PLANER,VIEWER}`, `type User`, `me`/`users`/`setUser`/`removeUser`), `graph/auth.go` (`authMiddleware` gegen `authProvider`-Interface → unit-testbar ohne DB; Dev-Fallback bei `auth.enabled=false`; 401 kein Header / 403 unbekannt; `UserFromContext`; `roleCanWrite`; `auditUser`), Resolver, Seed beim Boot in `NewPlexams`, Rollen-Enforcement (VIEWER read-only via `isDataChangingOperation`) + `server.production` (Playground/Introspection aus) in `graph/server.go`, Audit-`user` aus ctx-Principal (Fallback `operator.*`). Tests: `graph/auth_test.go` grün; build/vet/lint/test sauber. Deploy: `deploy/` (docker-compose mongo+plexams+apache, `apache/Dockerfile`+`plexams.conf` mod_auth_openidc-vHost, `.plexams.yaml.example`, `.env.example`, README mit **Redirect-URI `https://<host>/redirect_uri` für die Zentrale IT**), Dockerfile gehärtet (non-root `USER plexams`, EXPOSE 8080). **Offen:** Phase 1 Secrets-Umbau (Abschnitt 9, ZPA/Jira Client-pro-User, PAT-Verschlüsselung) NICHT gebaut; Phase 2 granulare `@requires`-Rollen NICHT gebaut; commit + plexams.gui-Anweisungen (`me`/Rolle anzeigen) noch offen; Websocket-ctx-Propagation für Subscriptions am echten Server verifizieren.

**Teil-Slice "Wer hat was gemacht" bereits umgesetzt (2026-07-08, working tree, noch nicht committet):** lokale `operator.name`/`operator.email`-Config → `Plexams.operator` (getrennt vom geteilten `planer`-Doc), `p.OperatorID()` (email>name); `user`-Feld an `MutationLogEntry` (Schema+DB+Model regeneriert) + `user`-Filter in `mutationLog`-Query; `mutationLogMiddleware` stempelt `entry.User = p.OperatorID()` (kein ctx nötig, prozess-weite Konstante); File-Uploads via neuem `p.LogUpload(ctx, name, kv...)` (Type "upload") in primuss-zip/email-attachment(+zip)/semester-dump/dataset(+csv). Funktioniert JETZT im lokalen Multi-Instanz-Setup mit geteilter Server-MongoDB. GUI muss noch `user`-Spalte/Filter in der mutationLog-Ansicht zeigen.

Ziel: plexams.go auf Server deployen mit Auth + Rollenkonzept.
- **Auth am Apache-Proxy (mod_shib, SAML-SP)** — Backend authentifiziert nicht selbst, vertraut Header `X-Remote-User` (= Shib `mail`-Attribut); Backend bindet auf 127.0.0.1, Proxy überschreibt Client-Header autoritativ.
- **Autorisierung MUSS im Backend erzwungen werden** (GUI ist nie Sicherheitsgrenze); plexams.gui nur kosmetisch (`me`-Query anzeigen, später Buttons ausblenden).
- **Rollen-Store:** neue `users`-Collection in globaler `plexams`-DB (analog `db/planer.go`), aus Config seedbar.
- **Jetzt zwei Planer mit Vollzugriff** (Rolle `PLANER`); granulare Rollen mittelfristig via gqlgen `@requires`-Directive.
- **Audit "wer":** Feld `user` an `MutationLogEntry` — der Chokepoint existiert schon (`graph/mutation_logging.go` → `mutation_log`), fehlt nur das Wer.
- **Wichtig:** `planer`-Doc (Email-Absenderidentität, [[emails-over-graphql]]) strikt getrennt halten von den neuen `users` (Operatoren/Logins).
- Phase 0 baut mit Dev-Fallback (`auth.devUser`), sodass lokaler Betrieb unverändert lauffähig bleibt.

**Secrets-Entscheidung (Weg A, Envelope-Encryption):** benutzerbezogene PATs (ZPA, Jira) verschlüsselt in der globalen DB (AES-256-GCM, `{keyVersion,nonce,ciphertext}`, keyed per User-Email); Master-Key (KEK) nur in Server-Config/Env, nie in DB/Git; GUI nur schreibend (`setUserSecret`, kein Getter); geteilte Secrets (SMTP) bleiben in der Datei. Struktureller Kern-Umbau: ZPA/Jira-Client von Singleton (`SetZPA`/`SetJira`) auf **Client-pro-User** (Token aus ctx-Principal). Regeln: PATs in `mutation_log`/Logs maskieren; `user_secrets` aus dump/dataset/csv-Export ausschließen; KEK fehlt → fail-closed. Details im Plan-Abschnitt 9.

Begleitende GUI-Änderungen wie immer als plexams.gui-agent-Anweisungen ausgeben (siehe [[gui-and-cli-sync]]).
