# Konfiguration (`.plexams.yaml`)

`plexams.go` lädt beim Start eine globale Konfigurationsdatei **`.plexams.yaml`**
(gesucht im aktuellen Verzeichnis `.` und im `$HOME`). Optional wird zusätzlich
eine per-Semester-Datei `<semester>.yaml` aus `semester-path` gemerged — die ist
inzwischen aber **optional**, weil die per-Semester-Konfiguration in der Datenbank
liegt (Collection `semester_config_input`, beim ersten Start automatisch aus der
YAML migriert) und über das GUI gepflegt wird.

Kurz: In die `.plexams.yaml` gehören **Bootstrap + Secrets + betriebliche Keys,
die (noch) nicht in der DB liegen**. Die eigentliche Semester-Planung (`from`,
`until`, `slots`, `forbiddenDays`, E-Mail-Adressen, MUC.DAI-Slots) kommt aus der DB.

---

## 1. Erforderlich (Bootstrap)

```yaml
semester: 2026-SS              # Pflicht
semester-path: ~/semester      # optional: nur nötig, wenn noch per-Semester-YAMLs benutzt werden

db:
  uri: mongodb://localhost:27013
  database: ""                 # optional, Default = Semestername (z. B. "2026-SS")
```

Der **Planer** (`planer.name`/`planer.email`) liegt inzwischen in der DB (global,
GUI-editierbar über `setPlaner`); der Config-Block ist nur noch Bootstrap/Fallback
für den ersten Start und kann danach entfallen:

```yaml
planer:                        # optional, sobald in der DB gesetzt
  name: Vorname Nachname
  email: planer@hm.edu
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
  noreplymail: noreply@hm.edu  # Reply-To für „bitte über JIRA antworten“-Mails (optional)
```

## 4. Anny (Raumbuchungen, nur lesend)

```yaml
anny:
  url: https://b.anny.eu/api/v1/bookings    # optional, Default ist genau das
  token: <anny-token>                       # Secret (Read-only)
  personalization_name:                     # ein Name ODER eine Liste
    - Oliver Braun
    - Michael Heinl
  rooms:                                     # nur diese Räume berücksichtigen
    - R1.046
    # ...
```

## 5. Sonstiges (Bootstrap)

```yaml
jira:
  url: https://jira.cc.hm.edu/servicedesk/customer/portal/13   # optional (Default gesetzt)

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
über das GUI (bzw. `plexams.go init`) gepflegt. Beim ersten Start werden sie
einmalig aus einer vorhandenen `<semester>.yaml` migriert; danach ist der
YAML-Block überflüssig:

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
