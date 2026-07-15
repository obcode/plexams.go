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
  hostname: plexams.cs.hm.edu  # FQDN für HELO/EHLO + Message-ID-Domain (optional; Default
                               # plexams.cs.hm.edu). Verhindert 554-Ablehnung durch eine
                               # Message-ID mit Container-Hostname (z.B. @docker-desktop).
  testmail: planer@hm.edu      # Ziel für Probeläufe (run=false)
  cc: smtp.cc@hm.edu           # filterbare Selbstkopie (Cc) bei echten Sends
  replymail: planer@hm.edu     # Reply-To für beantwortbare Mails (optional)
  noreplymail: noreply@hm.edu  # Reply-To-Adresse für „bitte über JIRA antworten“-Mails
                               # (optional; Default noreply+plexams@hm.edu, pro Planer in der
                               # GUI überschreibbar)
  noreplyname: "Prüfungsplanung FK07 (NOREPLY)" # Anzeigename der noreply-Reply-To (optional;
                               # Default s.o., pro Planer in der GUI überschreibbar)
  fromaddress: noreply@hm.edu  # Adresse, ALS die gesendet wird (From-Header), optional.
                               # Für Server, die nur Versand als authentifizierter Account
                               # erlauben (z.B. noreply@hm.edu ohne „Send-As" für den Planer):
                               # From wird diese Adresse, der Planer-NAME bleibt Anzeigename,
                               # die Planer-Adresse wird Reply-To. Leer = From = Planer.
  envelopefrom: noreply@hm.edu # SMTP-Envelope-Absender (MAIL FROM / Return-Path), optional.
                               # Meist derselbe Account wie fromaddress. Bounces gehen hierhin,
                               # SPF prüft diese Domain. Leer = fällt auf fromaddress zurück
                               # (bzw. From).
```

> **Hinweis (Versand über einen geteilten Account, z.B. `noreply@hm.edu`):** Viele Server
> (Exchange u.ä.) lehnen Mails mit `554 5.7.1 … does not meet our delivery requirements` ab,
> wenn der `From`-Header nicht zum authentifizierten `username` passt. Setze dann
> `fromaddress` (und i.d.R. `envelopefrom`) auf den Login-Account. Ergebnis:
> `From: "Planer Name" <noreply@hm.edu>`, `Reply-To: planer@hm.edu` — Empfänger sehen den
> Namen, Antworten erreichen den Planer.

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

## 4a. Nächtlicher Auto-Sync (`scheduler.*`)

Eine In-Process-Goroutine zieht **einmal täglich** ZPA (Prüfungen, Lehrende,
Aufsichtsbedarf) und die Anny-Buchungen für das **aktive Workspace-Semester** — dieselben
Importe wie die manuellen GUI-Aktionen, die unverändert bestehen bleiben — und mailt die
**Unterschiede** zum vorherigen Stand. Nutzt die `zpa.*`/`anny.*`/`smtp.*`-Konfiguration,
**keine** zusätzlichen Secrets. Nur fürs Server-Deployment gedacht; lokal ausgelassen.

```yaml
scheduler:
  enabled: true                                # Default false (aus)
  time: "03:00"                                # Uhrzeit lokal (Europe/Berlin), HH:MM; Default 03:00
  changesRecipient: pruefungsplanung-fk07@hm.edu  # Mail NUR bei Änderungen
  debugRecipient: ""                           # Mail bei JEDEM Lauf (Heartbeat, auch „keine
                                               # Änderungen"/Fehler); leer = aus
  # sources:                                   # optional; ganz weglassen = alle vier ziehen
  #   exams: true
  #   teachers: true
  #   invigilatorRequirements: true
  #   anny: true
```

> - **Erstbefüllung:** Ist eine Quelle beim ersten Lauf noch leer (z.B. ein frisches
>   Semester, für das ZPA noch keine Prüfungen liefert), erscheint alles als *neu*, sobald
>   ZPA erstmals Daten hat — dann feuert die Änderungs-Mail (bei sehr vielen Einträgen
>   kompakt als Zähler statt Einzelliste).
> - **Kollision:** Läuft gerade ein manueller Import/E-Mail-Versand, wartet der Nachtlauf
>   kurz und überspringt sonst sauber (derselbe Exclusive-Op-Guard).
> - **Sofort starten:** Über das GUI lässt sich der komplette Lauf on-demand auslösen
>   (`triggerScheduledSync`, gestreamt wie die anderen Importe).
> - Der Nachtlauf betrifft nur das **aktive** Workspace-Semester. Andere Semester
>   unabhängig vom aktiven Workspace zu beobachten wäre ein späterer Ausbau (eigene
>   gescopte Instanz pro Semester).

## 5. Jira (FK07-Prüfungsplanungs-Helpdesk)

Die on-prem-Jira `jira.cc.hm.edu` (Service-Desk-Projekt **FK07PP**) wird per
**Personal Access Token (PAT)** angebunden — `Authorization: Bearer <PAT>` gegen
die REST-API v2. Damit liest/erstellt plexams Tickets, kommentiert, ändert den
Status und hängt Dateien (PDF/CSV) an.

Das **PAT ist personenbezogen und pro Planer** — es liegt **nicht** in der YAML,
sondern **verschlüsselt in der DB** und wird pro Request aus der Identität des
Anmelders aufgelöst. Jeder Planer trägt sein PAT auf der **„Mein Account"-Seite** der
GUI ein (`setMyJiraToken`); es wird sofort AES-256-GCM-verschlüsselt gespeichert
(Master-Key `secrets.key`, s. Abschnitt 5c) und nie im Klartext zurückgegeben. In der
YAML bleiben nur die **geteilten, nicht-personenbezogenen** Werte:

```yaml
jira:
  baseurl: https://jira.cc.hm.edu     # Instanz-Wurzel (für die REST-Anbindung)
  project: FK07PP                     # Default-Projekt-Key (createJiraIssue, offene Issues)
  url: https://jira.cc.hm.edu/servicedesk/customer/portal/13   # Kundenportal-Link
                                      # (nur für den E-Mail-Helper `jiraURL`, optional)
  # token: <jira-pat>                 # ENTFÄLLT — PATs liegen jetzt pro Planer
                                      # verschlüsselt in der DB (Mein Account → setMyJiraToken)
```

> PAT anlegen: in Jira → Avatar → **Profil** → **Personal Access Tokens** →
> *Create token* — dann auf „Mein Account" eintragen. Ohne hinterlegtes PAT melden die
> Jira-Operationen für diesen Nutzer „kein Jira-Token hinterlegt". Ohne `secrets.key`
> (Abschnitt 5c) sind PAT-Operationen fail-closed. `url` (Kundenportal) ist unabhängig
> und speist nur den `jiraURL`-Platzhalter in den E-Mail-Vorlagen.

## 5a. Sonstiges (Bootstrap)

```yaml
server:
  port: "8080"                 # GraphQL-Server-Port
  production: false            # true: schaltet Playground + GraphQL-Introspection ab
                               # (für den Server-Betrieb). Default false (lokal an).
  allowedorigins:              # zusätzliche CORS-Origins (optional)
    - http://localhost:5173
```

## 5b. Auth / Rollen (Server-Deployment)

Beim Betrieb hinter einem Auth-Proxy (nginx + `oauth2-proxy`, OIDC gegen
`sso.hm.edu`) authentifiziert das Backend **nicht** selbst — es vertraut der
Identität, die der Proxy als Header setzt, und erzwingt die **Autorisierung** (Rollen)
selbst. Details + fertige Vorlagen: [`../deploy/`](../deploy/).

```yaml
auth:
  enabled: false               # false/leer = lokale Entwicklung: ein voll berechtigter
                               # Dev-User wird injiziert, nichts wird abgewiesen (lokaler
                               # Betrieb unverändert). true = Server: Header wird erzwungen.
  header: X-Remote-User        # Header mit der verifizierten Identität (E-Mail);
                               # muss zur Proxy-Konfig passen. Default X-Remote-User.
  displaynameHeader: X-Remote-Displayname   # optionaler Anzeigename-Header
  devuser: vorname.nachname@hm.edu          # nur lokal (auth.enabled=false): Audit-Identität
  seedusers:                   # Allow-Liste, nur beim ERSTEN Boot geseedet (solange die
                               # users-Collection leer ist); danach GUI-verwaltet (setUser).
                               # Mindestens EIN ADMIN nötig, sonst kann später niemand
                               # neue User über die GUI anlegen.
    - { email: planer1@hm.edu, name: Planer Eins, role: ADMIN }
    - { email: planer2@hm.edu, name: Planer Zwei, role: ADMIN }
```

> Rollen (ein User hat genau **eine**; Hierarchie **`ADMIN` ⊇ `PLANER` ⊇ `VIEWER`**):
> **`VIEWER`** = nur lesen + Validierungen (keine datenändernden Operationen);
> **`PLANER`** = volle Planung; **`ADMIN`** = alles wie PLANER **plus**
> Benutzerverwaltung (`setUser`/`removeUser`). ADMIN ist höherwertig — man ist nicht
> „PLANER *und* ADMIN", ADMIN schließt die PLANER-Rechte ein. Die Autorisierung wird im
> Backend erzwungen (Sicherheitsgrenze); die GUI passt sich nur kosmetisch an. `auth.*`
> ist strikt getrennt vom `planer`-Doc (geteilte E-Mail-Absenderidentität). Auf dem
> Server kommt die Audit-Identität (`mutation_log.user`) aus dem Proxy-Principal statt
> aus `operator.*`. Lokal (ohne `auth.enabled`) ist der Dev-User **ADMIN**.

## 5c. Secrets / Master-Key (`secrets.key`)

Benutzerbezogene Secrets (aktuell das **Jira-PAT pro Planer**) liegen **verschlüsselt
in der globalen `plexams`-DB** (Collection `user_secrets`, AES-256-GCM). Der
**Master-Key (KEK)** dafür lebt **nur** hier in der Config bzw. als Env-Var — **nie**
in der DB, **nie** in Git.

```yaml
secrets:
  key: <base64-32-byte-key>    # `openssl rand -base64 32`  (32 Byte → AES-256)
```

> Erzeugen: `openssl rand -base64 32`. Fehlt `secrets.key`, sind alle
> PAT-Operationen **fail-closed** (klare Fehlermeldung, kein Klartext-Fallback); ein
> **fehlerhafter** Key (kein base64 / ≠ 32 Byte) deaktiviert sie ebenfalls (Log-Fehler
> beim Start). Der Key gehört zu den gemounteten Secrets (`.plexams.yaml` / Env /
> Docker-Secret). PATs erscheinen nie im Klartext in Logs oder im `mutation_log`
> (maskiert) und sind aus Dump/Export ausgeschlossen. `keyVersion` wird mitgespeichert
> → spätere Key-Rotation ohne Big-Bang möglich.

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
