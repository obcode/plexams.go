---
name: auth-roles-shibboleth
description: "Planned deployment + role concept — Shibboleth (Apache mod_shib) auth, DB users collection, audit \"who did what\"; plan file persisted, build later."
metadata:
  node_type: memory
  type: project
  originSessionId: 96f9d929-9c15-41cd-b151-07bb987f0f76
---

Geplanter Umbau (Plan persistiert 2026-07-08 in `~/.claude/plans/ich-will-diese-anwendung-cached-shamir.md`, Start-Prompt steht am Ende der Plan-Datei).

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
