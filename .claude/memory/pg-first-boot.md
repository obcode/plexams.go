---
name: pg-first-boot
description: "Was der allererste Start gegen eine leere PostgreSQL-Datenbank tut — und warum drei ERR-Zeilen dort kein Fehler waren"
metadata:
  node_type: memory
  type: project
---

Für den Cut-over ([[postgres-migration]]). Am 2026-08-04 beim End-to-End-Durchlauf der
globalen Stammdaten gefunden und behoben (`0764c25`).

**Der Erststart ist ein eigener Zustand, den es unter Mongo nicht gab.** Dort legte das
erste Schreiben die Datenbank nebenbei an; hier braucht `active_semester` und
`semester_config_input` je eine Zeile in `semester`, die noch niemand angelegt hat.
Das Henne-Ei ist beabsichtigt aufgelöst: `--semester` pinnt eines, **damit der Server
überhaupt startet und `createSemester` aufgerufen werden kann.**

Genau diesen normalen Weg hat der Server mit **drei `ERR`-Zeilen** quittiert und dann
korrekt weitergearbeitet: zweimal der FK-Verstoß aus `RememberActiveSemester`, einmal
die fehlende Semesterkonfiguration. Die Ablehnung selbst ist richtig und durch
`TestActiveSemesterUnregisteredSemesterIsRejected` festgenagelt — **falsch war nur die
Meldung.** Jetzt: `ErrSemesterNotRegistered` (SQLSTATE 23503, erkannt wie in
`DeleteStudyProgram`), der Aufrufer unterscheidet es von einem echten Schreibfehler,
und die fehlende Konfiguration ist eine `WRN` mit Semester-Id und Handlungsanweisung.

**Das zählt, weil das Bootstrapping der Produktionsdatenbank der erste echte Lauf
dieses Pfads sein wird** — und niemand soll dabei rätseln, ob die Migration kaputt ist.

Die Reihenfolge, die funktioniert und beim Cut-over gilt:

1. `goose ... up` gegen die leere DB,
2. Server mit `--semester <YYYY-SS>` starten (Registry noch leer, das ist in Ordnung),
3. `createSemester` — registriert Semester **und** Config in einer Transaktion,
4. `setSemester` — der laufende Server bindet seine Config beim Start, ein frisch
   angelegtes Semester ist ihm sonst `compatible: false` mit `semesterConfig: null`,
   während `allSemesterNames` es schon als kompatibel führt. Kein Fehler, nur zwei
   verschiedene Quellen: die eine liest die DB, die andere den Prozesszustand.
5. Ab dem nächsten Start entfällt der Pin — `ResolveStartSemester` findet es selbst.

**Nebenbefund, gleiche Sitzung, eigener Commit (`023a727`):** `.gitignore` enthielt
`plexams.yaml`, die Datei heißt aber `.plexams.yaml` — das Muster hat sie nie
getroffen. In ihr stehen `secrets.key`, das ZPA-Passwort und die SMTP-Zugangsdaten;
zwischen einer lokalen Dev-Config und einem **öffentlichen** Repo stand allein der
gitleaks-Hook. Siehe [[pg-dev-setup]] für den Rest der lokalen Einrichtung.
