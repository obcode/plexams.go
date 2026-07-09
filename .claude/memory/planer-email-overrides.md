---
name: planer-email-overrides
description: "Sender identity overrides (envelope-from, testMail/cc, noreply addr+name) live GLOBALLY with the planer now; move to per-user under Shibboleth later."
metadata:
  node_type: memory
  type: project
  originSessionId: bd27a846-7d8e-4cb2-95fa-083e88a5b4e0
---

Email-Absender-Identität in `plexams/email/sender.go` + global `Planer` (DB `plexams`, collection `planer`):

- **EnvelopeFrom** (`smtp.envelopefrom`): SMTP MAIL FROM entkoppelt vom sichtbaren From. Sendet über geteilten Account (z.B. noreply@hm.edu, muss zum SMTP-Login passen), From bleibt der Planer. go-mail `GetSender` bevorzugt HeaderEnvelopeFrom, sonst From.
- **Defaults** (überschreibbar): testMail/cc = Planer-Email mit `+plexams` (oliver.braun@hm.edu → oliver.braun+plexams@hm.edu); noreplyMail = `noreply+plexams@hm.edu`; noreplyName = `"Prüfungsplanung FK07 (NOREPLY)"`. Precedence je Feld: Planer-Override → `smtp.*` config → hardcoded/derived default.
- **Overrides** liegen GLOBAL beim Planer (model.Planer: TestMail/Cc/NoreplyMail/NoreplyName als optionale Felder), gesetzt via `setPlaner`.

**Why global, not per-semester:** der Planer selbst ist global und trägt über Semester. Nutzer wollte den einfacheren Weg.

**How to apply / future (Shibboleth):** Wenn [[auth-roles-shibboleth]] kommt, sollen die zwei Planer leichtgewichtige DB-User werden; diese Override-Felder wandern dann zum User, und man setzt pro Semester einfach einen der User als Planer. Dann ist es effektiv doch "pro Semester" wählbar, ohne die Felder zu duplizieren. Bis dahin: global.

**Dry-run per-run override:** Session-Override (NICHT persistiert) auf dem Sender: `setDryRunTestMail`/`resetDryRunTestMail` Mutations + `dryRunTestMail` Query (status: override/current/default/overridden). GUI-Mailseite soll Banner zeigen wenn override ≠ default, und Button-Text "Probelauf (nur an <current>)".

**Message-ID / HELO:** go-mail leitete Message-ID + HELO aus `os.Hostname()` ab → im Container `@docker-desktop`. `smtp.hostname` (Default `plexams.cs.hm.edu`) setzt jetzt `mail.WithHELO` + `SetMessageIDWithValue(random@hostname)`. NB: die `docker-desktop`-Message-ID war NICHT die 554-Ursache (Server echot sie nur als „affected message ID" zur Referenz) — trotzdem korrekt.

**554-Ursache = From ≠ Login (Send-As-Policy):** `mail.it.hm.edu` (Login `noreply@hm.edu`) lehnt `From: oliver.braun@hm.edu` mit `554 5.7.1 does not meet our delivery requirements` ab, weil der Service-Account keine „Send-As"-Berechtigung für den Planer hat. Fix (Service-Account-Muster, vom Nutzer gewählt): From-ADRESSE = geteilter Account, From-NAME = Planer, Reply-To = Planer-Email. Config `smtp.fromaddress` (Resolution: fromaddress → envelopefrom → planerEmail); `envelopefrom` fällt auf fromaddress zurück. Explizites `fromaddress` erlaubt weiterhin From≠Envelope (Escape-Hatch). Leer = Legacy From=Planer.
