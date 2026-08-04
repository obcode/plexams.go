---
name: pg-global-master-data-import
description: "Wie die globale Mongo-DB `plexams` nach PostgreSQL kommt: tools/mongo2pg, was drinsteckt, und die drei Stellen, an denen die Echtdaten nicht zum Modell passen"
metadata:
  node_type: memory
  type: project
---

Für den Cut-over ([[postgres-migration]]). Gebaut am 2026-08-04, verifiziert gegen den
echten Dump.

**Die globalen Stammdaten sind der einzige Bestand, den niemand nachimportieren kann.**
Semester werden ausdrücklich *nicht* migriert (sie kommen aus ZPA/Primuss/Anny zurück),
Räume/NTAs/Studiengänge sind handgepflegt.

## Was tatsächlich drinliegt

Gemessen am archivierten Dump (`semester`-Repo, `mongo-backup/plexams/`):

| Collection | Dok. |
| --- | --- |
| `nta` | 86 |
| `rooms` | 37 |
| `study_programs` | 15 |
| `permanent_non_invigilators` | 10 |
| `planer` / `anny_config` | je 1 |
| `email_templates` | **0 — leer** |

Zusammen **150 Dokumente**. `users`, `user_secrets`, `generation_config` und
`scheduler_state` existieren **gar nicht** (deckt sich mit dem Befund aus Phase 3a).
`active_semester` (1 Dok.) baut sich beim nächsten Start selbst wieder auf.

**Alle sechs relevanten Collections hatten schon typisierte PG-Schreiber** — an `db/`
musste nichts dazukommen.

## Das Werkzeug

`tools/mongo2pg`, **eigenes Modul**, damit `mongo-driver` nicht ins Hauptmodul
zurückkehrt. Liest die **archivierten `.bson`-Dateien**, nicht eine laufende Mongo:
Generalprobe ohne VPN und ohne `mongod`, beliebig wiederholbar. Schreibt über die
`db.PG`-Methoden der Anwendung, nicht über selbstgebaute INSERTs. Idempotent.

`go build ./...` im Wurzelverzeichnis erfasst das Verzeichnis **nicht** — eigenes Modul.

## Drei Stellen, an denen die Echtdaten nicht zum Modell passen

- **`nta.group` ist der alte Name von `program`, kein unbekanntes Feld.** Kein Dokument
  hat beide (21 von 86 nur `group`, 59 nur `program`, 6 keins). Es als „Feld, das das
  Modell nicht mehr kennt" zu verwerfen hätte bei einem Viertel der NTAs den Studiengang
  verloren — und `program` ist `not null`. **Das ist die Umkehrung der Regel aus
  [[pg-db-layer-conventions]]:** dort waren zwei gleichnamige Felder *nicht* dasselbe
  (`studentreg.program` vs. `student_program`), hier sind zwei verschieden benannte
  Felder *dasselbe*. Beide Male hilft nur nachsehen, woher der Wert wirklich kam.
- **`study_programs.category = 'mucdai'`** ist die Altkategorie (3 von 15). Der Check
  erlaubt nur `fk07`/`joint`/`misc`, also muss die Umsetzung nach `joint` +
  `jointFaculty` **beim Import** passieren — `plexams.migrateMucdaiToJoint` käme zu spät,
  weil die Zeile gar nicht erst hineindarf.
- **`rooms.needsRequest` wird gelesen und weggeworfen** (generierte Spalte in PG). 9 von
  37 Räumen tragen *beide* Schreibweisen — das Speichern eines abgeleiteten Werts ist,
  was sie auseinanderlaufen ließ.

Unstrittig verworfen: `nta.exams` (0 von 86) und `nta.notForExams` (1 von 86, Ancode
243). Das Modell hat beide seit Jahren nicht, die laufende Code-Basis ignoriert sie
also ohnehin — der eine Ausschluss war längst wirkungslos.

## Die Falle, die das Werkzeug an sich selbst vorgeführt hat

**BSON-Arrays dekodieren zu `primitive.A`, nicht zu `[]any`.** `primitive.A` ist ein
*benannter* Typ mit Unterbau `[]interface{}` — eine Assertion auf `[]any` schlägt fehl
und liefert stillschweigend nichts. Der erste Trockenlauf meldete fröhlich
„anny_config: 1 importiert" und hätte eine **leere** Personalisierungsnamen-Liste
geschrieben; genau die entscheidet, welche Anny-Buchungen als unsere gelten.

Deshalb drehen die Tests ihre Fixtures durch `bson.Marshal`/`Unmarshal`, statt Go-Maps
von Hand zu bauen: Zahlen kommen als `int32`, Arrays als `primitive.A`, und ein
Konvertierer, der nur `int` und `[]any` kennt, sieht bis dahin korrekt aus.

**Und das ist der Grund für den Berichtsteil „Verworfene Felder":** jeder Schlüssel, den
kein Konvertierer angefasst hat, wird mit Anzahl gemeldet. Ein leerer Abschnitt heißt
„alles abgebildet oder bewusst ignoriert" — Stille ist hier der Fehlermodus, nicht die
Ausnahme.

## Verifikation

Gegen eine frisch migrierte Datenbank: 37/15/86/10/1/1 Zeilen, und
`places_with_socket` (8), `hmeb_seats` (12), `request_with <> 'NONE'` (9) sowie die
Sitzplatzsumme (704) stimmen exakt mit den unabhängig aus dem BSON gerechneten Werten
überein. Zweiter Lauf ändert nichts.

**Der Bericht nennt Matrikelnummern**, damit die betroffenen Datensätze in der GUI
auffindbar sind — er gehört nicht in Issues, Commits oder Chats. Echte Dumpdateien
bleiben im privaten `semester`-Repo ([[semester-repo]]).

## Nachzuarbeiten nach dem Import

- **6 NTAs ohne Studiengang** und **8 ohne Gültigkeitszeitraum** (Überschneidung: die 8
  sind die alten, unvollständigen Einträge). Sie werden mit Leerstring importiert und im
  Bericht genannt, statt übersprungen zu werden: eine übersprungene NTA ist ein
  Nachteilsausgleich, den jemand still nicht mehr bekommt. In der GUI nacharbeiten.
- Ein `group`-Wert ist Freitext (`Master Elektrotechnik`) statt eines Kürzels.
  `nta.program` hat keinen Fremdschlüssel, also geht es durch — aber es ist falsch.
