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

```yaml
zpa:
  baseurl: https://zpa.cs.hm.edu/rest
  username: <zpa-user>
  password: <zpa-passwort>
  token: <zpa-token>           # Secret — nicht teilen
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

Diese werden weiterhin aus der Config gelesen (global oder per-Semester-YAML).
Sie sollen perspektivisch in die DB wandern, sind aktuell aber hier nötig, wenn
die jeweiligen Funktionen genutzt werden:

```yaml
# Studiengänge der Kooperationen / Sonstige (Seed für die globale StudyProgram-Stammdaten)
mucdaiprograms: [DE, GS, ID]
miscprograms:   [GN]

# Räume
rooms:
  timelag: 15                  # Minuten Puffer zwischen Belegungen
roomconstraints:
  additionalseats: 0

# Aufsichten-Generator (Simulated Annealing)
invigilation:
  optimizer:
    tolerance: 60
    iterations: 1000000
    startTemp: 20000
    endTemp: 0.5
    weights:
      beyondTolerance: ...
      minuteBalance: ...
      coverage: ...
      distribution: ...
      daySpan: ...
      maxDays: ...
      preferExamDays: ...
      overTargetFactor: ...

# Datei-Ausgaben / Vorlagen
invigilationStats:
  dir: ...
  prefix: ...
coverPages:
  dir: ...
  prefix: ...

# Diverses
duration: 90                   # Default-Prüfungsdauer
donotpublish: [<ancode>, ...]  # diese Ancodes nicht ins ZPA hochladen
publish:
  additionalExams: [...]
knownConflicts:
  studentRegs: ...
specialInterests: [...]
```

> `invigilatorConstraints` wurde in die DB migriert (GUI-CRUD). Der YAML-Eintrag
> wird nur noch für die einmalige Erst-Migration gelesen und kann danach entfernt
> werden.

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

## 8. Entfallen — bitte **löschen**

Diese Keys gibt es nicht mehr; sie werden höchstens noch für die einmalige
Migration gelesen und können raus:

- `semesterConfig.fromFK07`   (es gibt nur noch `from`)
- `semesterConfig.dayNumberStart`
- `semesterConfig.goDay0`
- `goslots`                   (ersetzt durch `mucdaislots`, absolute Paare)
