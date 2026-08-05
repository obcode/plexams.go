# mongo2pg — Einmal-Import dessen, was niemand neu holen kann

Holt die **globale** MongoDB-Datenbank `plexams` und aus **einem** Semester die beiden
handgetippten Collections nach PostgreSQL. Einmalwerkzeug für den Cut-over; danach
löschen.

Der Rest eines Semesters wird **nicht** migriert — Prüfungen, Lehrende, Anmeldungen,
Konflikte und Anny-Buchungen kommen durch einen erneuten Import aus ZPA/Primuss/Anny
zurück. Unersetzlich ist nur, was jemand eingegeben hat.

## Aufruf

```sh
# immer erst trocken
go run . -uri "postgres://plexams@127.0.0.1:5433/plexams?sslmode=disable" \
         -dump          /pfad/zu/mongo-backup/plexams \
         -semester-dump /pfad/zu/mongo-backup/2026-WS \
         -dry-run

# dann echt
go run . -uri ... -dump ... -semester-dump ...
```

Beide Dumps sind einzeln optional; das Semester wird aus dem Verzeichnisnamen
abgeleitet (`-semester 2026-WS`, wenn er nicht passt).

Die Zieldatenbank darf **leer** sein: das Werkzeug migriert sie zuerst.

Der Lauf ist **idempotent**: jede Collection wird pro Datensatz upgesertet, ein
Wiederholungslauf ändert nichts. Ein abgebrochener Lauf kann einfach wiederholt werden.

## Warum aus dem Dump und nicht aus einer laufenden Mongo

Reproduzierbar, ohne VPN und ohne `mongod` — und der Dump ist ohnehin das Artefakt,
das laut Migrationsplan aufgehoben wird. Eine Generalprobe kostet damit nichts.

## Was importiert wird

| Collection | Bemerkung |
| --- | --- |
| `rooms` | beide Schreibweisen (`placesWithSocket`/`placeswithsocket`, `hmebSeats`/`hmebseats`) |
| `study_programs` | Altkategorie `mucdai` → `joint` + `jointFaculty` |
| `nta` | `group` → `program` (siehe unten) |
| `permanent_non_invigilators` | |
| `anny_config` | |

**Nicht importiert:** `active_semester` (baut sich beim nächsten Start neu auf),
`email_templates` (enthält nur Überschreibungen des Planers, im Dump leer) und
`planer` — den globalen Planer gibt es nicht mehr: der Default steht in `planer.*`
in der Serverkonfiguration, das Semester überschreibt ihn in der GUI.
`users`, `user_secrets`, `generation_config` und `scheduler_state` gibt es in der
globalen DB gar nicht — die Allow-Liste kommt aus `auth.seedusers`.

Aus dem **Semester-Dump** (`-semester-dump`):

| Collection | Bemerkung |
| --- | --- |
| `semester_config_input` | registriert nebenbei das Semester (das macht sonst `createSemester`) |
| `preplan_exams` | inklusive Constraints und der beiden Paar-Relationen |

Alles andere im Semester-Verzeichnis wird im Bericht unter **„Nicht importiert"**
aufgelistet, sofern es nicht leer ist. Das ist die Kontrolle für die Annahme, auf der
der Cut-over beruht: steht dort etwas Handgepflegtes (`constraints`,
`connected_exams`, ein fertiger Plan), stimmt sie für dieses Semester nicht.

### Zwei Formfehler, die nur die alten Daten zeigen

**Vor-slotless-Dokumente.** Eine Konfiguration mit `slots` statt `startTimes` und eine
Vorplanungs-Prüfung mit `planneddaynumber`/`plannedslotnumber` stammen aus der Zeit vor
dem slotless-Refactor. Die absoluten Zeiten stehen dort nirgends — sie lagen im Code
jener Version. Beides wird **gemeldet, nicht geraten**: die Prüfung kommt ungeplant an,
die Startzeiten fehlen und müssen in der GUI eingetragen werden.

**Einseitige Paare.** In MongoDB trug jede Seite von `notSameSlot`/`canShareSlot` ihre
eigene Liste, und nichts erzwang, dass beide übereinstimmen. In PostgreSQL ist ein Paar
**eine** kanonische Zeile, und das Schreiben einer Prüfung ersetzt ihre ganze Seite der
Relation — ein einseitiges Paar würde also vom Schreiben der Gegenseite wieder gelöscht.
Der Import ergänzt die Gegenrichtung vorher und meldet jede Ergänzung.

## Die vier Stellen, an denen die Daten nicht zum Modell passen

**`mucDaiAllowedTimes` würde stillschweigend verschwinden.** Das Feld trägt `json:"-"`,
die Spalte ist `jsonb` — das Modell zu serialisieren würde die reservierten Zeiten
wortlos wegwerfen. Der Import verteilt sie deshalb vorher auf alle joint-Studiengänge,
so wie `applyLegacyJointTimes` es zur Laufzeit tut. Deshalb müssen die Studiengänge
**vor** der Semesterkonfiguration importiert sein (im selben Lauf ist das automatisch
die richtige Reihenfolge).


**`nta.group` ist der alte Name von `program`, kein unbekanntes Feld.** Kein Dokument
hat beide; 21 von 86 haben nur `group`. Es als unbekannt zu verwerfen würde bei einem
Viertel der NTAs den Studiengang verlieren — und `program` ist `not null`.

**`study_programs.category = 'mucdai'`** ist die Altkategorie. Der Check erlaubt nur
`fk07`/`joint`/`misc`, also muss die Umsetzung beim Import passieren und nicht wie
sonst beim Start durch `plexams.migrateMucdaiToJoint`.

**`rooms.needsRequest` wird gelesen und weggeworfen.** In PostgreSQL ist es eine
generierte Spalte (`request_with <> 'NONE'`). Genau das Speichern hat die beiden
Schreibweisen auseinanderlaufen lassen: 9 von 37 Räumen tragen beide Schlüssel.

## Der Bericht

Am Ende stehen zwei Abschnitte:

- **Verworfene Felder** — jeder Schlüssel, den kein Konvertierer angefasst hat, mit
  Anzahl. Ein leerer Abschnitt heißt: alles ist entweder abgebildet oder bewusst
  ignoriert. Das ist der Sinn des Werkzeugs — nichts soll unbemerkt verschwinden.
- **Anmerkungen** — pro Datensatz, was beim Import entschieden wurde (Altfeld
  übernommen, Pflichtfeld leer, Kategorie umgesetzt).

**Der Bericht nennt Matrikelnummern**, damit die betroffenen Datensätze in der GUI
auffindbar sind. Er gehört damit nicht in Issues, Commits oder Chats.

## Tests

`go test ./...` — synthetische Fixtures, durch `bson.Marshal`/`Unmarshal` gedreht statt
als Go-Maps geschrieben. Das ist wesentlich: der Treiber liefert Arrays als
`primitive.A` und Zahlen als `int32`, und ein Konvertierer, der nur `[]any` oder `int`
kennt, sieht richtig aus, bis er auf echtes BSON trifft. Genau daran sind die
Anny-Personalisierungsnamen einmal stillschweigend leer importiert worden.

Echte Dumpdateien liegen im privaten `semester`-Repo und dürfen hier nicht hinein
(Matrikelnummern, Namen, E-Mail-Adressen).

## Eigenes Modul

`go.mod` ist getrennt, damit `go.mongodb.org/mongo-driver` nicht ins Hauptmodul
zurückkehrt, aus dem die Migration ihn gerade entfernt hat. `go build ./...` im
Wurzelverzeichnis erfasst dieses Verzeichnis deshalb nicht.
