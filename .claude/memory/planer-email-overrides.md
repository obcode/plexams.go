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

**Message-ID / HELO Fix:** go-mail leitete Message-ID + HELO aus `os.Hostname()` ab → im Container `@docker-desktop` → `mail.it.hm.edu` lehnte mit `554 5.7.1 does not meet our delivery requirements` ab (nach DATA, Envelope/Auth waren ok). Fix: `smtp.hostname` (Default `plexams.cs.hm.edu`, der reale Host) setzt jetzt `mail.WithHELO` + `SetMessageIDWithValue(random@hostname)`. Gilt für ALLE Sends, nicht nur Probeläufe.
