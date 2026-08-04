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

## Stufe 1 ist gebaut (2026-08-04)

**Beides läuft als Skript auf dem Host**, nicht als Route im Server —
`deploy/backup/pg-backup.sh` und `deploy/backup/pg-restore.sh`. Der Grund ist nicht
Bequemlichkeit: **`pg_restore` braucht die Datenbank ohne den Verbindungspool der
Anwendung.** Das Restore-Skript stoppt deshalb den `plexams`-Container, spielt ein
und startet ihn wieder. Eine Route im laufenden Server hätte ihren eigenen Pool
schließen und sich danach neu verbinden müssen — mehr bewegliche Teile für eine
seltene, ohnehin manuelle Operation.

Damit ist auch die Frage nach `pg_dump` im Produktions-Image **erledigt**: das
`plexams.go`-Image (alpine, nur ca-certificates + tzdata) braucht die Binaries
**nicht**. Beide Skripte rufen sie über `docker compose exec postgres` im
`postgres:18-alpine`-Container auf, wo sie zur Major-Version des Servers passen —
die Versionsregel (`pg_restore` darf neuer sein als der Server, nie älter) ist damit
automatisch erfüllt.

**Der Restore droppt die Datenbank und legt sie neu an**, statt `pg_restore --clean`
zu benutzen. Mit `--clean` plus `--exit-on-error` bricht ein harmloses „cannot drop"
den ganzen Lauf ab; ohne `--exit-on-error` sieht ein halb fehlgeschlagener Restore aus
wie ein gelungener. In eine frische Datenbank ist das Archiv entweder vollständig drin
oder es scheitert laut. Vorher nimmt das Skript einen Sicherheits-Dump des aktuellen
Standes — das falsche Archiv einzuspielen ist damit selbst rückgängig zu machen.

Gegen einen echten Cluster verifiziert: ein Ziel mit anderen Zeilen *und* einer
zusätzlichen Tabelle kommt als exakt das Archiv zurück, und `--force` beendet
tatsächlich eine offen gelassene Verbindung.

**Der Replica-Set-Aufwand entfällt ersatzlos** (Keyfile, `rs.initiate()`, uid 999,
~70 Zeilen README) — PostgreSQL kann Transaktionen ohne Zeremonie.

## Die offene Folge: `lastDumpAt` stempelt niemand mehr

`SetLastDumpAt` ist portiert und hat **keinen Aufrufer**. Früher stempelte der
Semester-Dump-Download in der GUI; den gibt es nicht mehr, und ein Cron-Job auf dem
Host kann es nicht (er müsste durch oauth2-proxy).

**Folge: die Backup-Erinnerung in der GUI (`hasUnsavedChanges`) leuchtet dauerhaft**
und lässt sich durch nichts zurücksetzen. Das ist jetzt ein Dauerzustand, kein
Übergang mehr. Zwei sinnvolle Auflösungen, noch nicht entschieden:
1. Die Erinnerung aus der GUI entfernen — sichern macht ohnehin die Maschine.
2. `lastDumpAt` beim **CSV-Export** stempeln (`/download/my-inputs-csv.zip`). Das ist
   die Sicherungshandlung, die dem *Planer* gehört, und damit wird der Hinweis wieder
   handlungsfähig: „seit deiner letzten Sicherung geändert". Der `pg_dump` ist die
   Sicherung der *Maschine* und ein anderer Vorgang.
- Die typisierten **CSV-Datensätze bleiben unabhängig davon bestehen**
  (`plexams/csv_export.go`, 10 Datensätze). Sie sind der einzige Weg, der die
  handgepflegten Eingaben *lesbar* und *semesterweise* sichert, und sie haben den
  Backendwechsel unverändert überstanden. Für „meine Eingaben retten" sind sie das
  richtige Werkzeug, nicht der Dump.

Nicht zu verwechseln mit der **Backup-Rotation des Hosts** (`make backup`,
fehlendes Offsite-Ziel) — die steht in [[deploy-topology]] und [[semester-repo]].
