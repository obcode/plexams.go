# mongo2pg — Einmal-Import der globalen Stammdaten

Holt die **globale** MongoDB-Datenbank `plexams` nach PostgreSQL. Einmalwerkzeug für
den Cut-over; danach löschen.

Semester werden **nicht** migriert — sie werden als Mongo-Dumps archiviert und aus
ZPA/Primuss/Anny neu importiert. Nur die globalen Stammdaten sind handgepflegt und
damit unersetzlich.

## Aufruf

```sh
# immer erst trocken
go run . -dump /pfad/zu/mongo-backup/plexams \
         -uri "postgres://plexams@127.0.0.1:5433/plexams?sslmode=disable" \
         -dry-run

# dann echt
go run . -dump ... -uri ...
```

Die Zieldatenbank muss migriert sein (`goose -dir db/migrations postgres "$URI" up`).

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
| `planer` | |
| `anny_config` | |

**Nicht importiert:** `active_semester` (baut sich beim nächsten Start neu auf) und
`email_templates` (enthält nur Überschreibungen des Planers, im Dump leer).
`users`, `user_secrets`, `generation_config` und `scheduler_state` gibt es in der
globalen DB gar nicht — die Allow-Liste kommt aus `auth.seedusers`.

## Die drei Stellen, an denen die Daten nicht zum Modell passen

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
