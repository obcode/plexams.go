---
name: pg-global-master-data-import
description: "Wie die globale Mongo-DB `plexams` und die zwei getippten Collections eines Semesters nach PostgreSQL kommen: tools/mongo2pg, was drinsteckt, und wo die Echtdaten nicht zum Modell passen"
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

## Ein Semester kommt doch mit — die zwei getippten Collections

Ergänzt am **2026-08-04**. Die Regel „Semester werden nicht migriert" bleibt richtig,
hat aber eine Ausnahme, die dieselbe Begründung hat wie die globalen Stammdaten:
**`semester_config_input` und `preplan_exams` hat jemand getippt.** Prüfungen,
Lehrende, Anmeldungen, Konflikte und Anny-Buchungen kommen durch einen erneuten
Import zurück, diese beiden nicht. Aufruf: `-semester-dump <dir>` (Semester aus dem
Verzeichnisnamen, sonst `-semester`).

`mongo2pg` **registriert das Semester dabei selbst** (`EnsureSemester`) — das ist der
Teil, den sonst `createSemester` macht; die Config danach in der GUI anzulegen würde
sie zweimal schreiben.

**Der Bericht listet jede nicht importierte, nicht leere Collection auf.** Das ist die
Kontrolle für die Annahme, auf der der ganze Cut-over steht: steht dort etwas
Handgepflegtes (`constraints`, `connected_exams`, ein fertiger Plan), stimmt sie für
dieses Semester nicht — sichtbar **vor** dem Umschalten, nicht erst, wenn es in der
GUI fehlt.

Drei Dinge, die die Form der Daten erzwingt:

- **`mucDaiAllowedTimes` verschwindet lautlos, wenn man nichts tut.** Das Feld trägt
  `json:"-"`, die Spalte ist `jsonb` — das Modell zu serialisieren wirft die
  reservierten Zeiten wortlos weg. Sie werden beim Import auf alle
  joint-Studiengänge verteilt, wie `applyLegacyJointTimes` es zur Laufzeit tut.
  **Deshalb müssen die Studiengänge vorher drin sein** (im selben Lauf automatisch).
  Der Schemakommentar in `00002` verlangt genau das; 2026-WS hat 30 Einträge dort
  und keine `jointProgramAllowedTimes`.
- **Vor-slotless-Dokumente werden gemeldet, nicht geraten.** Eine Konfiguration mit
  `slots` statt `startTimes`, eine Vorplanungs-Prüfung mit `planneddaynumber` — die
  absoluten Zeiten lagen im Code jener Version, nicht in den Daten. Die Prüfung kommt
  **ungeplant** an statt gar nicht; Modul, Prüfer, Erwartungszahl und Constraints
  sind ja weiterhin gültig. Der archivierte `Test26SS`-Dump ist genau so ein
  Bestand und damit die Generalprobe für diesen Zweig — **nicht** für die Form, die
  2026-WS haben wird.
- **Einseitige Paare.** `notSameSlot`/`canShareSlot` trugen in Mongo beide Seiten
  ihre eigene Liste, ohne Zwang zur Übereinstimmung. In PG ist ein Paar **eine**
  kanonische Zeile, und das Schreiben einer Prüfung ersetzt ihre ganze Seite — ein
  einseitiges Paar würde vom Schreiben der Gegenseite wieder gelöscht. Der Import
  ergänzt die Gegenrichtung vorher und meldet jede Ergänzung. Aus demselben Grund
  läuft er in **zwei Durchgängen**: erst alle Prüfungen ohne Paare (der Fremdschlüssel
  zeigt auf die andere Prüfung), dann die Paare.

## Nachzuarbeiten nach dem Import

- **6 NTAs ohne Studiengang** und **8 ohne Gültigkeitszeitraum** (Überschneidung: die 8
  sind die alten, unvollständigen Einträge). Sie werden mit Leerstring importiert und im
  Bericht genannt, statt übersprungen zu werden: eine übersprungene NTA ist ein
  Nachteilsausgleich, den jemand still nicht mehr bekommt. In der GUI nacharbeiten.
- Ein `group`-Wert ist Freitext (`Master Elektrotechnik`) statt eines Kürzels.
  `nta.program` hat keinen Fremdschlüssel, also geht es durch — aber es ist falsch.
