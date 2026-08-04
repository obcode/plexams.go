---
name: pg-backup-restore
description: "Backup/Restore nach der PostgreSQL-Migration: vorerst nur ganze Datenbank (pg_dump/pg_restore); Einzelsemester-Export später und darf teuer sein"
metadata:
  node_type: memory
  type: project
---

Entschieden am **2026-08-04** von Oliver, nachdem der Flip auf PostgreSQL durch war
([[postgres-migration]]).

**Das Semester-Backup/Restore wird nicht dringend gebraucht.** Es war unter MongoDB
gratis, weil ein Semester eine Datenbank war — das Zurückspielen eines Semesters war
schlicht eine Datenbank-Operation. In PostgreSQL teilen sich alle Semester ein Schema
mit `semester_id`, damit wäre es eine Zeilen-Operation über ~64 Tabellen. Diese Kosten
sind der Grund, warum es aufgeschoben wird, nicht ein fehlender Bedarf.

**Was gebaut wird — Stufe 1, jetzt:**
- **Backup = alles.** `pg_dump -Fc` über die ganze Datenbank, alle Semester zusammen.
- **Restore = alles.** `pg_restore` der ganzen Datenbank.

Kein Semesterfilter auf keiner der beiden Seiten. Das ist bewusst gröber als das, was
Mongo konnte, und für den Zweck „vor der Planungssitzung sichern" ausreichend.

**Was später kommt — Stufe 2:**
Ein Semester **einzeln herausnehmen und neu einspielen**. Das **darf ausdrücklich
teurer sein** (Laufzeit wie Implementierungsaufwand) — es ist der seltene Fall
(Testworkspace bauen, ein Semester auf eine andere Instanz ziehen), nicht der
Alltagsfall.

## Was beim Bauen von Stufe 1 zu bedenken ist

- **Der Restore der ganzen Datenbank kann nicht aus dem laufenden Prozess in die
  eigene Verbindung hinein passieren.** `pg_restore` braucht die Zieldatenbank
  exklusiv; der Server hält aber einen `pgxpool` darauf. Entweder läuft der Restore
  außerhalb der Anwendung (Skript/Host), oder die Anwendung muss ihren Pool schließen,
  neu anlegen lassen und sich danach neu verbinden. **Vor der Implementierung
  entscheiden** — das ist die eigentliche Schwierigkeit, nicht das Dumpen.
- `pg_dump`/`pg_restore` sind Binaries, keine Bibliothek. Sie liegen im
  `plexams.dev`-Image (`/usr/lib/postgresql/18/bin`) und müssen auch im
  Produktions-Image liegen, wenn die Route serverseitig ausgeführt wird — siehe
  [[deploy-topology]].
- **Versionsregel:** `pg_restore` darf neuer sein als der Server, umgekehrt nicht.
  Client und Server auf demselben Major halten (aktuell 18, siehe [[pg-dev-setup]]).
- `SetLastDumpAt` ist portiert und hat seit dem Löschen von `plexams/dump.go`
  **keinen Aufrufer**. Der Download der Gesamtsicherung ist die Stelle, an der der
  Stempel wieder gesetzt gehört — bis dahin steht die Backup-Erinnerung in der GUI
  dauerhaft, weil `hasUnsavedChanges` nie zurückgesetzt wird.
- Die typisierten **CSV-Datensätze bleiben unabhängig davon bestehen**
  (`plexams/csv_export.go`, 10 Datensätze). Sie sind der einzige Weg, der die
  handgepflegten Eingaben *lesbar* und *semesterweise* sichert, und sie haben den
  Backendwechsel unverändert überstanden. Für „meine Eingaben retten" sind sie das
  richtige Werkzeug, nicht der Dump.

Nicht zu verwechseln mit der **Backup-Rotation des Hosts** (`make backup`,
fehlendes Offsite-Ziel) — die steht in [[deploy-topology]] und [[semester-repo]].
