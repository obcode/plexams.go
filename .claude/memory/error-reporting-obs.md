---
name: error-reporting-obs
description: Das Paket obs/ — Fehler-Telemetrie nach GlitchTip mit fail-closed Scrubber, caller-Fingerprint, pseudonymem Nutzer; Phase 3a fertig und lokal gegengetestet (2026-08-05)
metadata:
  type: project
---

Phase 3a des Monitoring-Plans (`plexams.dev/.claude/plans/monitoring-plattform.md`) ist
umgesetzt und lokal gegen ein echtes GlitchTip geprüft. Vier Commits auf `main`, in dieser
Reihenfolge, weil **der Scrubber vor allem stehen muss, was senden kann**:

1. `feat(obs): add a fail-closed scrubber for error reports` — nur `scrub.go` + Tests.
2. `feat(obs): initialise error reporting and pseudonymise the user` — `obs.go`, `user.go`.
3. `feat(bootstrap): report errors when sentry.dsn is configured` — die Verdrahtung.
4. `feat(graph): report panics and give them the request they happened in`.

## Die zwei Entscheidungen, aus denen alles folgt

**Positivliste, keine Sperrliste.** `sentryzerolog` macht *jedes* unbekannte zerolog-Feld
zu einem Tag. Der Bestand loggt `mtknr` 33×, `name` 24×, `email` 10× — auch auf
Error-Ebene, also genau dort, wo gemeldet wird. Eine Sperrliste müsste gegen jede künftige
Logzeile nachgepflegt werden. Also `allowedTags` (`caller`, `semester`, `program`,
`ancode`, `room`, …) und alles andere fliegt. Genauso Header, Contexts und
Breadcrumb-Daten. Der Test, der das beweist, erfindet einen Tag-Schlüssel, den es nirgends
gibt, und prüft, dass er nicht durchkommt. Siehe [[pii-in-fehlerberichten]].

**Fingerprint auf `caller`.** `sentryzerolog` baut den Stacktrace *innerhalb* seiner
`Write`-Methode: die obersten Frames sind für alle ~640 Logstellen identische
zerolog-Interna, die Standardgruppierung würfe also Unzusammenhängendes zusammen. Nur
Logzeilen (`event.Logger == "zerolog"`) bekommen den Fingerprint; selbst gefangene Panics
haben einen echten Stack und behalten die Standardgruppierung.

## Was beim Bauen anders kam als im Plan

- **`event.Extra` gibt es in sentry-go v0.48 nicht mehr.** Der Plan sagt „Extra hart auf
  nil"; an seine Stelle treten `Contexts` (Positivliste `device`/`os`/`runtime`/`trace`).
- **Breadcrumbs werden nicht hart genullt**, sondern gefiltert — sonst wäre der bewusste
  Breadcrumb aus `mutation_logging.go` (Feldname + ancodes) gleich wieder weg.
- **`recoverFunc` bleibt**, statt durch `obs.RecoverFunc` ersetzt zu werden. Es ruft
  `obs.CapturePanic` auf und markiert seine eigene Logzeile mit `obs.SkipField`, damit der
  Panic nicht zweimal ankommt — einmal mit echtem Stack, einmal mit zerolog-Interna. Der
  zerolog-Fix von 2026-08-05 bleibt dadurch erhalten.
- **`recoverMiddleware` ist neu** (in `graph/recover.go`). `sentryhttp` allein wäre hier
  falsch: mit `Repanic: false` schluckt es den Panic und `net/http` liefert eine leere
  **200**, mit `true` stirbt die Verbindung ohne Status. Die eigene Middleware loggt,
  meldet und antwortet mit 500. `sentryhttp` bleibt als *erste* Middleware, aber für seine
  eigentliche Aufgabe: den Request-Hub.

## Fallstricke, die Zeit kosten würden

- **`CapturePanic` muss aus der `defer`-Funktion gerufen werden, die `recover()` gemacht
  hat.** Nur solange `runtime.gopanic` auf dem Stack liegt, reicht `sentry.NewStacktrace()`
  bis zur Stelle, die gecrasht ist. Ein Aufruf einen Sprung später zeigt auf den
  Fehlerbehandler. Ein Test hält das fest (`TestCapturePanicIsUnhandledAndCarriesAStack`).
- **`obs.Init` vor `newPlexams()`**, sonst gehen dessen vier `log.Fatal` (DB nicht
  erreichbar, kein brauchbares Semester, Tippfehler im `--semester`) verloren. zerolog
  flusht auf Fatal-Ebene synchron vor `os.Exit` — geprüft, das Event kommt an.
- **`sentry.CurrentHub()` an `NewWithHub`**, nicht `sentryzerolog.New`: sonst zwei Clients,
  zwei `BeforeSend`, zwei Puffer, und ein `Flush` erwischt die Hälfte.
- **`Config.transport` ist unexportiert** und existiert nur für die Tests: damit läuft ein
  echtes `Init` gegen `sentry.MockTransport`, die Tests decken also die ganze Kette
  zerolog → Writer → `BeforeSend` → Transport ab, nicht nur den Scrubber.
- **`caller` ist repo-relativ** (`graph/nta.resolvers.go:44`), seit
  `bootstrap.repoRelativeCaller` `zerolog.CallerMarshalFunc` setzt. zerologs Voreinstellung
  ist der absolute Pfad der *Build*-Maschine — im Container also ein anderer als hier, und
  weil der Fingerprint genau auf diesem Wert sitzt, landete ein lokal nachgestellter Fehler
  in einem anderen Issue als der Produktionsfehler, den er erklären sollte. `selfPath` in
  `bootstrap.go` benennt dafür die eigene Datei; wird sie umbenannt, fällt es auf den
  absoluten Pfad zurück — deshalb hält ein Test in `bootstrap_test.go` fest, dass
  `callerPrefix` nicht leer ist.

## Gegentesten

GlitchTip lokal aus `obcode/monitoring` (privat), im DevContainer unter Docker-in-Docker:
`glitchtip/.env` aus `.env.example` mit `GLITCHTIP_DOMAIN=http://localhost:8000`,
`EMAIL_URL=consolemail://`, `ENABLE_USER_REGISTRATION=true`, dann `docker compose up -d`.
Projekt und DSN ohne UI über `docker compose exec -T web ./manage.py shell`
(`apps.organizations_ext.models`, `apps.projects.models`, `ProjectKey.get_dsn()`).

**Achtung:** das benannte Volume `monitoring_glitchtip-db` überlebt einen frischen Klon,
die `.env` mit dem Passwort nicht (gitignored). Passt beides nicht zusammen, meldet der
`migrate`-Container einen `PoolTimeout` und die Ursache steht allein im Postgres-Log
(`password authentication failed`). `docker compose down -v` und neu.

Der schärfste Testfall, der auch tatsächlich lief: eine `addNTA`-Mutation mit vollem
Datensatz (Name, Mailadresse, Matrikelnummer) in einem Resolver zum Absturz gebracht. Im
Event: pseudonymer Nutzer, drei erlaubte Header, Breadcrumb `addNTA`, Stack bis in den
Resolver — und **keiner** der vier persönlichen Werte irgendwo im Payload.

Offen aus Phase 3: `3b` (die GUI) und danach der Rest des Plans.
