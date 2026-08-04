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

## Die dritte Richtung: Produktion → Entwicklungsrechner

Ergänzt am **2026-08-04**. Neben „sichern" und „zurückspielen" gibt es den Fall, um
den es im Alltag wirklich geht: *etwas stimmt in der Produktion nicht, und ich will
denselben Stand lokal unter dem Debugger haben.* Dafür gibt es
`plexams.dev/scripts/pg-prod.sh` (privates Repo, weil dort der Hostname steht):

```
pg-prod.sh pull    # pg_dump über ssh, restore in eine frische lokale DB, db.uri umstellen
```

**`pg_dump` läuft im postgres-Container auf dem Host, nicht durch einen Tunnel.**
`ssh … docker compose exec -T postgres pg_dump` löst die Zugangsdaten im Container
auf — sie erreichen keine lokale Prozessliste —, braucht keinen weitergeleiteten
Port, und die `pg_dump`-Version passt per Konstruktion zur Server-Major. Genau die
Bauform, die `pg-backup.sh` auf dem Host schon hat. Der Tunnel bleibt richtig für
interaktives Arbeiten (psql, DBeaver).

Eingespielt wird **immer in eine frische Datenbank**, aus demselben Grund, aus dem
`pg-restore.sh` droppt und neu anlegt: ein Restore über bestehende Daten sieht auch
dann gelungen aus, wenn er halb fehlschlug. Lokal ist das billig — eine Datenbank
mehr im Cluster, und `pg-dev.sh use` zeigt das Backend darauf (siehe
[[pg-dev-setup]]).

Die Archive liegen im privaten `semester`-Repo unter `pg-dumps/` und sind
**gitignored**: sie enthalten Matrikelnummern, Namen und NTA-Angaben. Das ist die
Lehre aus den eingecheckten Mongo-Dumps, die genau das in voller Historie stehen
haben ([[semester-repo]]).

## Zwei Sicherungen, zwei Adressaten

**Das ist die Unterscheidung, die den Entwurf trägt** (2026-08-04 entschieden):

| | wer sichert | was | wo |
| --- | --- | --- | --- |
| **Maschine** | Cronjob auf dem Host | ganze Datenbank, alle Semester | `pg-backup.sh` |
| **Planer** | Klick in der GUI | die handgepflegten Eingaben, lesbar als CSV | `/download/my-inputs-csv.zip` |

`lastDumpAt` wird beim **CSV-Export** gestempelt, nicht beim `pg_dump`. Nicht als
Notlösung: der Cronjob *könnte* es gar nicht — er müsste durch oauth2-proxy. Und die
Backup-Erinnerung in der GUI (`hasUnsavedChanges`) richtet sich seit jeher an den
Planer. Sie bedeutet damit wieder etwas, worauf er reagieren kann: „seit *deiner*
letzten Sicherung geändert".

Wer den Stempel je an den `pg_dump` hängen will, verschiebt still die Bedeutung des
Hinweises — dann meldet er „gesichert", obwohl der Planer nichts getan hat und seine
Eingaben nirgends lesbar liegen.
- Die typisierten **CSV-Datensätze bleiben unabhängig davon bestehen**
  (`plexams/csv_export.go`, 10 Datensätze). Sie sind der einzige Weg, der die
  handgepflegten Eingaben *lesbar* und *semesterweise* sichert, und sie haben den
  Backendwechsel unverändert überstanden. Für „meine Eingaben retten" sind sie das
  richtige Werkzeug, nicht der Dump.

Nicht zu verwechseln mit der **Backup-Rotation des Hosts** (`make backup`,
fehlendes Offsite-Ziel) — die steht in [[deploy-topology]] und [[semester-repo]].
