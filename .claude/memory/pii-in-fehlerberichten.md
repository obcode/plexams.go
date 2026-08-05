---
name: pii-in-fehlerberichten
description: Warum Fehler-Telemetrie in plexams eine Positivliste braucht — mtknr steht in 33 Logfeldern, in URLs und in GraphQL-Variablen
metadata:
  type: project
---

Beim Entwurf der Fehler-Telemetrie (2026-08-05, siehe den Plan in `plexams.dev`) sind drei
Leckwege aufgefallen, die alle vor der ersten gesendeten Zeile geschlossen sein müssen.

**1. Der zerolog→Sentry-Writer macht jedes Logfeld zu einem Tag.**
`sentryzerolog` schreibt unbekannte Felder ungeprüft nach `event.Tags`. Der Bestand loggt
`mtknr` 33×, `name` 24×, `email` 10× — darunter auf **Error**-Ebene, z. B.
`plexams/rooms.go:245`: `log.Error().Str("mtknr", *mtknr)`. Eine Sperrliste müsste man
ewig nachpflegen; es braucht eine **fail-closed Positivliste** (`caller`, `semester`,
`program`, `ancode`, `room`, … — und sonst nichts).

**2. Die URL ist manchmal die Matrikelnummer.** `plexams.gui` hat die Route
`src/routes/nta/[mtknr=string]`, verlinkt aus `src/lib/nta/NtaTR.svelte:22`. Nicht über
die NavBar erreichbar, aber von beiden NTA-Übersichten einen Klick entfernt. SvelteKit
schickt URLs in Transaktionsnamen und Breadcrumbs mit, also braucht es `beforeSend`
**und** `beforeBreadcrumb`. Offen: ob die Detailseite überhaupt noch gebraucht wird —
sie zu löschen wäre besser als jede Redaktion.

**3. GraphQL-Argumente.** `graph/mutation_logging.go` `flattenArgs` erzeugt genau die
mtknr/name/email-Paare, die in der DB richtig aufgehoben sind und in einem Fehlerbericht
nicht. Auch `oc.RawQuery` kann Inline-Literale tragen. Beides bleibt draußen; im
`recoverFunc` (`graph/recover.go`) ist das bereits so umgesetzt und durch einen Test
abgesichert.

**Folge fürs Logging:** deshalb gibt es seit 2026-08-05 `log.format: json`
(`bootstrap/reconfigureLogging`). Nicht aus Bequemlichkeit — ein Log-Versender kann nur in
JSON **nach Feldnamen** filtern (`"mtknr":"…"`); auf Fließtext bliebe das Raten nach
Wertformen, und `\b\d{7,10}\b` erwischt genauso Epoch-Zeitstempel und Dauern.

**Loki bekommt mehr zu sehen als Sentry**: Sentry nur Error+, Loki auch
`log.Debug().Str("mtknr", …)` und die Validierungsberichte auf Info-Ebene. Deshalb werden
Anwendungslogs erst nach einem Audit versendet, Infrastrukturlogs sofort.

Zum Gegenprüfen gibt es `plexams.dev/scripts/fake-ingest.py`: nimmt Events entgegen und
druckt die rohen Envelopes, `--grep` beendet sich mit 1, sobald ein Muster durchkommt.
