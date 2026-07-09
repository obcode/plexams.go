# Konfiguration (`.plexams.yaml`)

`plexams.go` lädt beim Start eine einzige globale Konfigurationsdatei
**`.plexams.yaml`** (gesucht im aktuellen Verzeichnis `.` und im `$HOME`). Eine
per-Semester-Datei `<semester>.yaml` wird **nicht mehr** gelesen — die
per-Semester-Konfiguration liegt vollständig in der Datenbank (Collection
`semester_config_input`) und wird über das GUI gepflegt.

Kurz: In die `.plexams.yaml` gehören **Bootstrap + Secrets + betriebliche Keys,
die (noch) nicht in der DB liegen**. Die eigentliche Semester-Planung (`from`,
`until`, `slots`, `forbiddenDays`, E-Mail-Adressen, MUC.DAI-Slots) kommt aus der DB.

---

## 1. Erforderlich (Bootstrap)

```yaml
semester: 2026-SS              # optional (Pin); ohne Angabe wird beim Start das
                               # zuletzt aktive bzw. neueste kompatible Semester gewählt

db:
  uri: mongodb://localhost:27013   # Pflicht
  database: ""                 # optional, Default = Semestername (z. B. "2026-SS");
                               # als Pin/Override z. B. für einen Replay-Klon "2026-SS-Test"
```

> Einzige echte Pflicht ist `db.uri`. `semester` ist nur noch ein optionaler Pin:
> ist es nicht gesetzt (und kein `db.database`-Pin), startet plexams mit dem zuletzt
> im GUI aktiven Semester, sonst mit dem neuesten kompatiblen. Umschalten geht zur
> Laufzeit über `setSemester` (GUI).

Der **Planer** (`planer.name`/`planer.email`) liegt inzwischen in der DB (global,
GUI-editierbar über `setPlaner`); der Config-Block ist nur noch Bootstrap/Fallback
für den ersten Start und kann danach entfallen:

```yaml
planer:                        # optional, sobald in der DB gesetzt
  name: Vorname Nachname
  email: planer@hm.edu
```

Der **Operator** (`operator.name`/`operator.email`) ist die *lokale* Identität der
Person, die diese plexams.go-Instanz betreibt (einer der Prüfungsplaner). Anders als
`planer.*` — die geteilte, für alle identische Absenderidentität aus der globalen DB —
kommt der Operator **nur aus dieser lokalen Config** und wird auf jeden Eintrag im
`mutation_log` (inkl. File-Uploads) gestempelt, damit im gemeinsamen Log erkennbar ist,
**wer was gemacht hat**. Jeder Planer trägt in seiner eigenen `.plexams.yaml` seine
eigene Identität ein. Ist nichts gesetzt, bleibt das `user`-Feld leer.

```yaml
operator:                      # lokal pro Planer/Instanz; nicht in der DB
  name: Vorname Nachname
  email: vorname.nachname@hm.edu
```

## 2. ZPA (Import/Upload)

Authentifizierung: **entweder** `token` **oder** `username`+`password`. Ist ein
`token` gesetzt, wird er direkt benutzt (username/password werden ignoriert);
sonst werden username/password gegen `/api-token-auth` eingetauscht.

```yaml
zpa:
  baseurl: https://zpa.cs.hm.edu/rest
  token: <zpa-token>           # Secret — entweder das ...
  # username: <zpa-user>       # ... oder username + password
  # password: <zpa-passwort>
  fk07programs:                # FK07-Studiengänge — Bootstrap/Seed (s. u.)
    - IF
    - IB
    # ...
  oldprograms:                 # ausgelaufene Studiengänge — Bootstrap/Seed (s. u.)
    - IC
```

> `zpa.fk07programs`/`zpa.oldprograms` werden zur Laufzeit aus den
> `StudyProgram`-Stammdaten (DB) abgeleitet, sobald diese existieren:
> aktuelle = `category fk07 && !retired`, alte = `category fk07 && retired`.
> Die Config-Listen dienen nur noch als Bootstrap und als Seed-Quelle für
> `seedStudyProgramsFromConfig` (oldprograms werden dabei als `retired` angelegt).
> Sind die Stammdaten gepflegt, können die beiden Listen aus der YAML entfallen.

## 3. SMTP / E-Mail

```yaml
smtp:
  server:
    name: postout.lrz.de
    port: 587
  username: <smtp-user>
  password: <smtp-passwort>    # Secret
  testmail: planer@hm.edu      # Ziel für Probeläufe (run=false)
  cc: smtp.cc@hm.edu           # filterbare Selbstkopie (Cc) bei echten Sends
  replymail: planer@hm.edu     # Reply-To für beantwortbare Mails (optional)
  noreplymail: noreply@hm.edu  # Reply-To-Adresse für „bitte über JIRA antworten“-Mails
                               # (optional; Default noreply+plexams@hm.edu, pro Planer in der
                               # GUI überschreibbar)
  noreplyname: "Prüfungsplanung FK07 (NOREPLY)" # Anzeigename der noreply-Reply-To (optional;
                               # Default s.o., pro Planer in der GUI überschreibbar)
  envelopefrom: noreply@hm.edu # SMTP-Envelope-Absender (MAIL FROM / Return-Path), vom
                               # sichtbaren From entkoppelt (optional). Erlaubt Versand
                               # über einen geteilten Account (muss zum SMTP-Login passen),
                               # während der From die Planer-Adresse bleibt. Bounces gehen
                               # hierhin, SPF prüft diese Domain. Leer = From wird als
                               # Envelope-Absender verwendet.
```

## 4. Anny (Raumbuchungen, nur lesend)

Nur der **Token** muss in die YAML; `url` ist optional (Default gesetzt). Es werden
**alle** Buchungen im Zeitraum geholt und gespeichert (so sieht man im GUI, wer wann
was gebucht hat).

```yaml
anny:
  url: https://b.anny.eu/api/v1/bookings    # optional, Default ist genau das
  token: <anny-token>                       # Secret (Read-only) — Pflicht für Anny
```

> `anny.personalization_name` und `anny.rooms` werden **nicht mehr** benötigt:
> - die Namen, die „unsere" Buchungen markieren (`mine`), liegen in der DB und sind
>   über das GUI setzbar (`setAnnyPersonalizationNames`); die YAML dient nur noch
>   als einmaliger Seed.
> - die relevanten Räume sind die Räume mit `requestWith: ANNY` in den globalen
>   Raum-Stammdaten (DB). Die YAML-Liste entfällt.

## 5. Jira (FK07-Prüfungsplanungs-Helpdesk)

Die on-prem-Jira `jira.cc.hm.edu` (Service-Desk-Projekt **FK07PP**) wird per
**Personal Access Token (PAT)** angebunden — `Authorization: Bearer <PAT>` gegen
die REST-API v2. Damit liest/erstellt plexams Tickets, kommentiert, ändert den
Status und hängt Dateien (PDF/CSV) an. Der PAT ist ein **Secret** und bleibt in
der Datei (nie in der DB).

```yaml
jira:
  baseurl: https://jira.cc.hm.edu     # Instanz-Wurzel (für die REST-Anbindung)
  token: <jira-pat>                   # Secret — Personal Access Token
  project: FK07PP                     # Default-Projekt-Key (createJiraIssue, offene Issues)
  url: https://jira.cc.hm.edu/servicedesk/customer/portal/13   # Kundenportal-Link
                                      # (nur für den E-Mail-Helper `jiraURL`, optional)
```

> PAT anlegen: in Jira → Avatar → **Profil** → **Personal Access Tokens** →
> *Create token*. Ohne `baseurl`/`token` bleibt die Jira-Anbindung inaktiv
> (der Start schlägt nicht fehl; nur die Jira-Operationen melden dann einen Fehler).
> `url` (Kundenportal) ist unabhängig davon und speist nur den `jiraURL`-Platzhalter
> in den E-Mail-Vorlagen.

## 5a. Sonstiges (Bootstrap)

```yaml
server:
  port: "8080"                 # GraphQL-Server-Port
  allowedorigins:              # zusätzliche CORS-Origins (optional)
    - http://localhost:5173
```

---

## 6. Betriebliche Keys — **noch** in der YAML (nicht in der DB)

Diese werden weiterhin aus der Config gelesen. Sie sollen perspektivisch noch in
die DB wandern, sind aktuell aber hier nötig, wenn die jeweiligen Funktionen
genutzt werden:

```yaml
# Datei-Ausgaben / Vorlagen (lokale Pfade — bleiben vorerst in der Datei)
invigilationStats:
  dir: ...
  prefix: ...
coverPages:
  dir: ...
  prefix: ...

# Bekannte/akzeptierte StudentReg-Konflikte (in der Validierung unterdrückt)
knownConflicts:
  studentRegs: ...
```

> `invigilatorConstraints` wurde in die DB migriert (GUI-CRUD). Der YAML-Eintrag
> wird nur noch für die einmalige Erst-Migration gelesen und kann danach entfernt
> werden.

> `mucdaiprograms`/`miscprograms` und `externalExamsBase.<prog>` werden nur noch
> von `seedStudyProgramsFromConfig` als Seed gelesen; zur Laufzeit kommt alles aus
> den `StudyProgram`-Stammdaten. Nach dem Seeden können sie aus der YAML entfallen.

> `invigilation.optimizer.*`, `rooms.timelag` werden nur noch als Default-Seed
> gelesen, solange keine `generationConfig` in der DB gespeichert ist (s. u.).

---

## 7. In die DB gewandert — **nicht mehr** nötig in der YAML

Diese per-Semester-Werte kommen aus der DB (`semester_config_input`) und werden
über das GUI gepflegt (`createSemester` / `setSemesterConfigInput`). Eine
`<semester>.yaml` wird nicht mehr gelesen:

- `semesterConfig.from`, `semesterConfig.until`
- `semesterConfig.slots`
- `semesterConfig.forbiddenDays`
- `semesterConfig.emails.*` (profs, lbas, lbaslastsemester, fs, sekr, roommanagement, kdp, lbaba)
- `semesterConfig.additionalexamer`
- `mucdaislots` (MUC.DAI-Slots als absolute `[Tag, Slot]`-Paare)

Außerdem (über eigene Collections / GUI gepflegt, YAML nur noch Seed/Fallback):

- `mucdaiprograms` / `miscprograms` → `StudyProgram`-Stammdaten (Kategorie)
- `externalExamsBase.<prog>` → Feld `externalExamsBase` am `StudyProgram`
- `duration` (Dauer-Overrides pro Ancode) → `setExamDuration` (greift nur bei ZPA-Dauer 0)
- `donotpublish` → Constraint `doNotPublish` an der Prüfung
- `roomconstraints.additionalseats` → Constraint `additionalSeats` (RoomConstraints)
- `rooms.timelag` + `invigilation.optimizer.*` → globale `generationConfig` (DB)
- `specialInterests` → `upsertSpecialInterest` (DB)
- `publish.additionalExams` → `upsertAdditionalExam` (DB)

## 8. Entfallen — bitte **löschen**

Diese Keys gibt es nicht mehr; sie werden höchstens noch für die einmalige
Migration gelesen und können raus:

- `semesterConfig.fromFK07`   (es gibt nur noch `from`)
- `semesterConfig.dayNumberStart`
- `semesterConfig.goDay0`
- `goslots`                   (ersetzt durch `mucdaislots`, absolute Paare)
