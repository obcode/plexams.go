---
name: planer-email-overrides
description: "Sender identity (planer + envelope-from, testMail/cc, noreply addr+name) hängt seit 2026-08-05 am SEMESTER; Default kommt aus planer.* in der Serverkonfiguration."
metadata:
  node_type: memory
  type: project
  originSessionId: bd27a846-7d8e-4cb2-95fa-083e88a5b4e0
---

Email-Absender-Identität in `plexams/email/sender.go` + `Planer` (Tabelle `semester_planer`, eine Zeile pro Semester):

- **EnvelopeFrom** (`smtp.envelopefrom`): SMTP MAIL FROM entkoppelt vom sichtbaren From. Sendet über geteilten Account (z.B. noreply@hm.edu, muss zum SMTP-Login passen), From bleibt der Planer. go-mail `GetSender` bevorzugt HeaderEnvelopeFrom, sonst From.
- **Defaults** (überschreibbar): testMail/cc = Planer-Email mit `+plexams` (oliver.braun@hm.edu → oliver.braun+plexams@hm.edu); noreplyMail = `noreply+plexams@hm.edu`; noreplyName = `"Prüfungsplanung FK07 (NOREPLY)"`. Precedence je Feld: Planer-Override → `smtp.*` config → hardcoded/derived default.
- **Overrides** liegen beim Planer DES SEMESTERS (`db.SemesterPlaner`: TestMail/Cc/NoreplyMail/NoreplyName), gesetzt via `setSemesterPlaner`, verworfen via `resetSemesterPlaner`.

**Zwei Ebenen (seit 2026-08-05, Migration 00008):** `planer.*` in der Serverkonfiguration ist der Default und NICHT zur Laufzeit editierbar; die Tabelle `semester_planer` überschreibt ihn pro Semester (GUI: `/config`). Die frühere globale DB-Zeile `planer` ist weg — mit genau einem Deployment hatte sie keine Aufgabe, die die Config nicht schon erfüllt, und drei Orte zum Nachsehen sind zwei zu viele, wenn eine From-Adresse falsch ist.

**Why per-semester (ersetzt das frühere „global ist einfacher"):** der globale Planer war, wer den Job GERADE hat — ein Mail-Nachversand für ein abgeschlossenes Semester hat es umetikettiert. `SwitchSemester` löst den Planer neu auf, ein Semesterwechsel wechselt also auch die Absenderidentität.

**Name+Email sind EINE Identität:** beide oder keines (Check `semester_planer_identity`, nochmal geprüft in `SetSemesterPlaner` und in der GUI). Nur die Adresse zu überschreiben würde als „Prüfungsplanung FK07 <jemand.anders@hm.edu>" rausgehen. Die vier Sender-Overrides mergen dagegen einzeln. Merge-Logik ist als reine Funktion `mergePlaner` testbar; `Planer.Inherited` sagt der GUI, ob sie den Default als Platzhalter statt als Wert zeigen soll.

**How to apply / future (Shibboleth):** Wenn [[auth-roles-shibboleth]] weiter ausgebaut wird, könnte `semester_planer` statt Freitext auf `app_user(email)` zeigen — dann wählt man den Planer des Semesters aus der Nutzerliste. Bewusst nicht gemacht, solange der Name eine Rolle („Prüfungsplanung FK07") und keine Person ist.

**Dry-run per-run override:** Session-Override (NICHT persistiert) auf dem Sender: `setDryRunTestMail`/`resetDryRunTestMail` Mutations + `dryRunTestMail` Query (status: override/current/default/overridden). GUI-Mailseite soll Banner zeigen wenn override ≠ default, und Button-Text "Probelauf (nur an <current>)".

**Message-ID / HELO:** go-mail leitete Message-ID + HELO aus `os.Hostname()` ab → im Container `@docker-desktop`. `smtp.hostname` (Default `plexams.cs.hm.edu`) setzt jetzt `mail.WithHELO` + `SetMessageIDWithValue(random@hostname)`. NB: die `docker-desktop`-Message-ID war NICHT die 554-Ursache (Server echot sie nur als „affected message ID" zur Referenz) — trotzdem korrekt.

**554-Ursache = From ≠ Login (Send-As-Policy):** `mail.it.hm.edu` (Login `noreply@hm.edu`) lehnt `From: oliver.braun@hm.edu` mit `554 5.7.1 does not meet our delivery requirements` ab, weil der Service-Account keine „Send-As"-Berechtigung für den Planer hat. Fix (Service-Account-Muster, vom Nutzer gewählt): From-ADRESSE = geteilter Account, From-NAME = Planer, Reply-To = Planer-Email. Config `smtp.fromaddress` (Resolution: fromaddress → envelopefrom → planerEmail); `envelopefrom` fällt auf fromaddress zurück. Explizites `fromaddress` erlaubt weiterhin From≠Envelope (Escape-Hatch). Leer = Legacy From=Planer.
