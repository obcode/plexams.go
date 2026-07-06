# Umbauplan: Slot-frei, rein zeitbasiert (Stufe 1)

Status: Entwurf / Planung. Ziel dieses Dokuments ist ein datei-genauer Umbauplan
für den Wechsel von einem slot-/tagnummern-basierten Modell zu einem rein
zeitbasierten Modell, plus die daraus folgende Liste an GUI-Änderungen.

## 0. Ziel & Scope

**Problem:** Platzierungen werden ordinal als `(DayNumber, SlotNumber)` gespeichert
und die absolute Zeit erst zur Laufzeit aus `from` + Slot-Startzeiten abgeleitet
(`plexams/semester_config.go` `deriveSemesterConfig`). Ändert sich der
Prüfungszeitraum, verschiebt sich die Bedeutung *jeder* gespeicherten Nummer.

**Zielbild:** Absolute Zeit ist die gespeicherte Wahrheit. Slots/Tag-Nummern
verschwinden aus den Daten. Konflikte, Räume und Aufsichten werden über
Zeitintervalle definiert statt über Slot-Buckets.

**Randbedingungen (vom Planer festgelegt):**
- CLI wird mittelfristig komplett entfernt → im Umbau slot-bezogene CLI-Teile
  **löschen**, nicht anpassen.
- **Keine Datenmigration** — sauberer Schnitt ab neuem Semester.
- Zeitregeln ersetzen Slot-Arithmetik (sameSlot/adjacent → „Überschneidung / zu
  nah"; Raum/Aufsicht → Turnaround-Puffer).

### Stufen (Repräsentation ≠ Verhalten trennen!)

- **Stufe 1 (dieses Dokument): Repräsentationswechsel bei gleicher Granularität.**
  Erlaubte Startzeiten = exakt die heutigen Slot-Startzeiten. Damit lässt sich per
  Golden-Test absichern, dass „gleicher Input → gleichwertiger Plan" gilt. Volle
  Zeitbasis, aber keine neue Granularität.
- **Stufe 2 (später, separat): Granularität + Kapazität.** Feinere/freie
  Startzeiten (z.B. jede volle/halbe Stunde), Raumkapazität und -Turnaround im
  Solver. Erst *nach* Stufe 1, sonst ist eine Planänderung nicht mehr auf ihre
  Ursache zurückführbar.

---

## 1. Neues Datenmodell

### 1.1 Platzierung (`graph/model/plan.go`)

Heute:
```go
type PlanEntry struct {
    DayNumber    int
    SlotNumber   int
    ExternalTime *time.Time
    Ancode       int
    Locked       bool
    PhaseFixed   bool
}
```

Neu (Wahrheit = absolute Zeit; keine Nummern mehr persistiert):
```go
type PlanEntry struct {
    Starttime *time.Time // nil = noch nicht platziert
    Ancode    int
    Locked    bool
    PhaseFixed bool
}

// Platziert?
func (pe *PlanEntry) IsPlanned() bool { return pe.Starttime != nil }
```
- `ExternalTime` entfällt als eigenes Feld — eine externe Prüfung ist einfach eine
  mit `Starttime`, die außerhalb des Zeitraums liegen darf. Das räumt zugleich die
  heutige Inkonsistenz weg (manche Exporte lesen `ExternalTime`, andere nicht).
- `DayNumber`/`SlotNumber` gibt es nicht mehr — weder persistiert noch abgeleitet.
  Alle Stellen, die heute Tag/Slot als Map-Key nutzen, wechseln auf `Starttime`
  bzw. Kalendertag (`time.Time`, auf Mitternacht normalisiert).

### 1.2 Config (`graph/model/semester_config_input.go`, `semesterconfig.graphqls`)

- **`Slots []string` → `StartTimes []string`** (`"HH:MM"`), umgedeutet als
  „erlaubte Anfangszeiten". Konsistent auch in plexams.gui umbenennen. Für Stufe 1
  identisch belegt wie heute.
- **`MucDaiSlots [][]int` → `MucDaiAllowedTimes []time.Time`.** Die heutige
  Festlegung ist faktisch nur „Vormittag vs. Nachmittag". Zukünftig (verkürzte
  Sommertage) wird das mit den MUC.DAI-Planenden neu ausgehandelt und wird eher
  „erlaubte / nicht erlaubte Zeiten". Modell daher als **Liste erlaubter
  Anfangszeiten** für MUC.DAI-Prüfungen (offen für spätere allow/deny-Erweiterung).
  `deriveMucDaiSlots` entfällt bzw. wird zu „Zeiten → Kandidaten".
- **`TimelagMin` und `ExamGapMinutes` in die SemesterConfig.** `TimelagMin`
  (Turnaround Raum/Aufsicht) wandert aus `GenerationConfig` (heute `rooms.timelag`,
  `generation_config.go:56`) in die SemesterConfig. `ExamGapMinutes` liegt schon
  dort. Beide sind semester-strukturelle Parameter, keine reinen Solver-Tuning-Werte.
- **`NotTooCloseMinutes` neu in SemesterConfig** (Default 120): Schwelle der
  „zu nah"-Warnung (siehe 2.1).

Die abgeleitete `SemesterConfig` wird zu einer reinen **Kandidaten-Erzeugung**:
Liste der erlaubten `time.Time`-Startzeitpunkte (`Tag × StartTimes`, Wochenenden/
`ForbiddenDays` raus), plus die **Tagesliste** `days: [Time!]`. Kein `Slot`-Typ mit
`DayNumber/SlotNumber` mehr.

**Tagesliste kommt aus dem Backend.** Die GUI stellt zwar nur `from..until` als
Eingabe, aber die konkrete Liste der Prüfungstage liefert das Backend
(`SemesterConfig.days`). So lässt sich z.B. „Samstage künftig nutzen" allein im
Backend ändern, ohne die GUI anzufassen.

---

## 2. Zeitbasierte Regeln (die eigentliche Vereinfachung)

### 2.1 Konflikt zweier Prüfungen eines Studierenden

Ersetzt `conflictcalc.SlotProximity` und den Slot-Scan in `validate.go` sowie die
`overrunsFor`-Logik in `examplan_build.go`.

Für zwei Prüfungen `a`, `b` desselben Studierenden mit Intervallen
`[start, start + dauer(+NTA)]`:
- **Überschneidung** (hart): Intervalle überlappen, oder Lücke `< ExamGapMinutes`.
- **Zu nah** (Warnung): Lücke `< NotTooCloseMinutes` (SemesterConfig, Default 120)
  am selben Tag.
- **Selber Tag** (Info): gleicher Kalendertag, sonst weit genug.
- **Folgetag** (Info): `|Δ| ≈ 24 h`.

Für die GUI kann aus dem Zeitabstand weiterhin ein Label abgeleitet werden
(`OVERLAP`/`TOO_CLOSE`/`SAME_DAY`/`NEXT_DAY`), aber die Wahrheit ist der
Zeitabstand. NTA-Verlängerung fließt direkt in `dauer` ein (heute schon in
`examplan_build.go` `ntaExt` vorhanden).

### 2.2 Raumbelegung

Ersetzt Slot-Buckets (`rooms_planned` mit `day/slot`) und die „Folgeslot
gesperrt"-Maschinerie (`rooms_for_slots.go` `restrictedSlotsForEXaHMRooms`).

- Ein Raum ist frei für ein Intervall `[start, ende]`, wenn kein anderes belegtes
  Intervall des Raums es überlappt, inkl. Turnaround: `[start - TimelagMin,
  ende + TimelagMin]`.
- „Geblockter Raum" (`rooms_blocked`) wird von `(room, day, slot)` zu einem
  Zeitintervall `(room, from, until, reason)`.

### 2.3 Aufsichten

`plexams/invigplan/` ist bereits zeit-nativ (`Position{Start, End()}`, `TimeSpan`,
`DayTimeWindow.AllowsTime`) und `GenerationConfig.TimelagMin` existiert schon als
Aufsichts-Puffer. Umbau = die Slot-Rückrechnung am Rand entfernen:
- `invigilation_generate.go` baut `Position`/`slotStart` heute aus
  `semesterConfig.Slots` (`[2]int{day,slot} → Starttime`). Neu direkt aus den
  Prüfungs-`Starttime`.
- `OwnExamSlots`/`OwnExamDays` (Slot-/Tag-Keys) → Zeitintervalle bzw. Kalendertage.
- „Nicht im Folgeslot" fällt weg — ergibt sich aus Intervall + `TimelagMin`.

---

## 3. Solver (`plexams/examplan/`)

Kernaussage: Die SA-Maschinerie ist generisch über eine Kandidatenliste; „Slots"
sind schon nur Indizes (`SlotOf []int`). Änderungen:

- **Kandidaten** (`examplan_build.go:55-67`): statt aus dem starren `sc.Slots`-
  Raster aus den erlaubten Startzeiten bauen. `examplan.Slot`/`SlotRef` behalten das
  `Start time.Time`-Feld (existiert bereits, `problem.go:21`), verlieren aber
  `Day int, Slot int` — Tages-/Nachbarschaftslogik wird über `Start` gerechnet.
- **`closeness()`/`farness()`** (`problem.go:410-448`): rein zeitbasiert. Same-day
  = gleicher Kalendertag aus `Start`; „adjacent"/„overlap" = Zeitabstand-Schwelle
  statt `SlotNumber±1`.
- **`nextSlot`/`prevSlot`** (`problem.go:267-305`, für NTA-Overrun): ersetzt durch
  „Kandidaten am selben Tag, deren Intervall bei Δ-Dauer überlappt".
- **`dayOfSlot`/Interior-Hole** (`problem.go`, `model.go:180-205`): Tag aus `Start`
  ableiten; Loch-Term bleibt konzeptuell gleich.
- **Startzeit-Strafe** (`examplan_time.go`): hängt schon an `Start.Hour()` — bleibt,
  wird durch mehr Kandidaten in Stufe 2 nur wirkungsvoller. Sommer-Vormittags-Ziel
  profitiert direkt.
- **Rückschreiben** (`examplan_build.go:662-682`): statt `AddExamToSlot(a, day,
  slot)` künftig `Starttime` des gewählten Kandidaten in den `PlanEntry` schreiben.

---

## 4. Datei-für-Datei-Umbau

### graph/model
- `plan.go` — `PlanEntry` neu (siehe 1.1). `InSlot()` → `IsPlanned()`.
- `semester_config_input.go` — `MucDaiSlots [][]int` → `MucDaiTimes []time.Time`;
  ggf. `Slots` → `StartTimes`.
- `rooms.go` — `PlannedRoom{Day,Slot}` → `{From,Until}` (oder `Start` + `Duration`);
  `UnplacedExam{Day,Slot}` → `{Start}`; `BlockedRoom{Day,Slot}` → `{From,Until}`.
- `models_gen.go` (generiert) — folgt aus Schema-Änderungen via `go generate`.
- `exam.go` — `PlannedExam` bezieht sich auf `PlanEntry` (kein Feld-Change nötig).
- `preplan_exam.go` — `PlannedDayNumber/PlannedSlotNumber *int` → `PlannedTime *time.Time`.

### db
- `plan.go` — bson-Felder `daynumber/slotnumber` raus; speichern/lesen über
  `starttime`. `AddExamToSlot` → `SetExamTime(ancode, *time.Time)`.
  `GetPlanEntriesInSlot`/`ExamsInSlot(day, time)` → Zeit-/Tag-Abfragen
  (`ExamsOnDay(date)`, `ExamsOverlapping(from, until)`).
- `rooms.go` / `rooms_unplaced.go` / `rooms_blocked.go` — Filter `{"day","slot"}` →
  Zeit-/Intervall-Filter; `PlannedRoomsInSlot` → `PlannedRoomsOverlapping`.
- `invigilation.go` — `Invigilation.Slot` (heute `Slot{DayNumber,SlotNumber,
  Starttime:zero}`) → `Start time.Time` + `Duration`. `PrePlannedInvigilation
  {Day,Slot}` → `{Start}`. Filter analog.
- `preplan_exams.go` — Felder `planneddaynumber/plannedslotnumber` → `plannedtime`.
- `database.go` — `MigrateLegacySemesterConfigInput`/`absoluteSlotPairs` **löschen**
  (keine Migration mehr). `SemesterConfigInput`-Persistenz auf neue Felder.

### plexams (Business-Logik)
- `plexams.go` — `GetStarttime`/`getSlotTime`/`getSlotForTime` **löschen** (kein
  Slot↔Zeit-Adapter mehr nötig); `allSlots` → `allStarttimes []time.Time`.
- `semester_config.go` — `deriveSemesterConfig` erzeugt Kandidaten-Startzeiten statt
  `Slot`-Grid; `deriveMucDaiSlots` **löschen**; `absoluteMucDaiSlots`/
  `weekdayDayNumber`/`intPairsFromViper("goslots")` **löschen**.
- `plan.go` — `AddExamToSlot`/`PreAddExamToSlot` **löschen** (Slot-basiert);
  `AddExamToSlottime`/`SetExternalExamTime` bleiben als *einziger* Schreibpfad,
  vereinfacht (kein `getSlotForTime` mehr, direkt `Starttime`).
- `conflictcalc/conflictcalc.go` — `SlotProximity` → `TimeProximity(a, b start,
  dauerA, dauerB, gapMin)`. Labels/`ProximityRank` anpassen.
- `validate.go` — Slot-Scan (`p[i].DayNumber==… && SlotNumber±1`) → Zeitabstand-
  Regel (2.1). `getSlotTime`-Aufrufe entfernen.
- `examplan_build.go` — Kandidaten aus Startzeiten; `overrunsFor`/`blockMin`-Logik
  durch Intervall-Overlap ersetzen; Rückschreiben auf `Starttime`.
- `examplan/problem.go`, `model.go` — zeitbasierte `closeness/farness/nextSlot`.
- `rooms*.go` (`rooms.go`, `roomsPrepare.go`, `rooms_for_slots.go`,
  `rooms_blocked.go`, `rooms_free_seats.go`, `request_rooms.go`,
  `room_requests_generate.go`) — Slot-Iteration → Zeit-Intervalle; Turnaround.
  `rooms_for_slots.go` (RoomsForSlot-Cache + EXaHM restricted slots) entfällt
  weitgehend.
- `invigilation_generate.go`, `invigilators.go`, `invigilation.go`,
  `ics_invigilator.go`, `invigcalc/`, `invigplan/` — Slot-Keys → Zeit.
- Exporte (`ics.go`, `zpa_post.go`, `csv.go`, `csv_export.go`, `csvgen/`,
  `pdfDraft*.go`, `pdfgen/`, `export.go`, `email_*.go`, `email/`) — statt
  `getSlotTime(day,slot)` direkt `planEntry.Starttime` verwenden. Die
  `slotTime func(day,slot)`-Closures entfallen; Signaturen der Sub-Pakete
  (`csvgen`, `pdfgen`, `email`) vereinfachen sich.
- `preplan_*.go` (`preplan_connect.go`, `preplan_assign.go`, `preplan_booking.go`,
  `preplan_overview.go`, `preplan_exams.go`) — `plannedTime` statt day/slot.

### graph (Schema + Resolver)
- `plan.graphqls` — `Slot`/`PlanEntry`/`Starttime`/`ExamDay` überarbeiten; Queries
  `examsInSlot(day,time)`/`preExamsInSlot` → `examsAt(start: Time!)` bzw.
  `examsOnDay(date: Time!)`; `allowedSlots`/`awkwardSlots` → `allowedStarttimes`.
- `room.graphqls` — `RoomsForSlot`, `plannedRoomsInSlot(day,time)`,
  `blockRoomForSlot(day,slot)`, `SlotInput`, `PlannedRoom{day,slot}`,
  `BlockedRoom{day,slot}`, `UnplacedExam{day,slot}`,
  `roomsWithFreeSeatsForSlot(day,time)` → zeitbasiert (`start: Time!` /
  `from/until`).
- `invigilation.graphqls` — `roomsWithInvigilationsForSlot(day,time)`,
  `invigilator(room,day,time)`, `prePlanInvigilation(day,slot)`,
  `PrePlannedInvigilation{day,slot}`, `Invigilation.slot` → zeitbasiert.
- `preplan_exam.graphqls` — `setPreplanExamSlot(dayNumber,slotNumber)` →
  `setPreplanExamTime(start: Time)`; `PreplanExam.plannedDayNumber/…` →
  `plannedTime`.
- `preplan_overview.graphqls` — `PreplanSlotNeed{dayNumber,slotNumber,starttime}`
  → nur `starttime`.
- `semesterconfig.graphqls` — `SemesterConfigInput.mucDaiSlots [[Int]]` →
  `mucDaiTimes [Time!]`; ggf. `slots` → `startTimes`; `SemesterConfig` neue Form.
- `exam_schedule.graphqls` — `ExamScheduleDiagnostics` Felder (`sameSlot`,
  `maxSlotSeats`, `slotsUsed`, `maxExamsPerSlot`) → zeitbasierte Pendants
  (`overlaps`, `maxSeatsAt`, `starttimesUsed`, `maxExamsAt`).
- Resolver (`*.resolvers.go`) — Signaturen folgen; `planEntryResolver.Starttime`
  wird trivial (Feld statt Ableitung); nach Schema-Änderung `go generate ./...`.

### cmd (LÖSCHEN, nicht anpassen)
- `cmd/plan.go` (291 Z.) — enthält `add-exam-to-slot`, `pre-add-exam-to-slot`,
  `add-exam-to-slottime` u.a.: **löschen** (Funktion zieht in GUI-Mutations um).
- `cmd/rooms.go`, `cmd/invigilation.go` — slot-bezogene Kommandos **löschen**.
- Weitere `cmd/*.go`, die über gelöschte `plexams`-Methoden laufen, mit-entfernen.
- Übergreifend: `docs/cli-to-gui-migration-plan.md` beachten — dieser Umbau ist
  ein großer Schritt dieser Migration.

---

## 5. Was komplett verschwindet (Netto-Löschung)

- `plexams.go`: `GetStarttime`, `getSlotTime`, `getSlotForTime` (+ `allSlots`).
- `semester_config.go`: `deriveMucDaiSlots`, `absoluteMucDaiSlots`,
  `weekdayDayNumber`, goslots-Migration.
- `db/database.go`: `MigrateLegacySemesterConfigInput`, `absoluteSlotPairs`.
- `conflictcalc`: Slot-Differenz-Arithmetik.
- `examplan_build.go`: `blockMin`/`slotBlockDuration`/`overrunsFor` → ein
  Intervall-Overlap.
- `rooms_for_slots.go`: RoomsForSlot-Cache + `restrictedSlotsForEXaHMRooms`
  (Adjazenz-Sonderfall).
- Modell: `Slot{DayNumber,SlotNumber}`, `SlotInput`, `ExamDay.Number`.
- CLI: `cmd/plan.go` und slot-Teile aus `cmd/rooms.go`, `cmd/invigilation.go`.

---

## 6. Test-Strategie

Da es keine Migration gibt, sind Golden-Tests **innerhalb** des neuen Modells zu
verankern, plus Property-Tests für die neuen Regeln.

1. **Regel-Unit-Tests (neu):**
   - Konflikt: Overlap/„zu nah"/„selber Tag"/„Folgetag" über Zeit + Dauer + NTA,
     inkl. Grenzfälle exakt = `ExamGapMinutes`.
   - Raum: Intervall-Overlap mit `TimelagMin` (Turnaround) — belegt/frei.
   - Aufsicht: Position-Overlap + `TimelagMin`; `DayTimeWindow`-Grenzen.
2. **Solver-Golden-Snapshot:** ein fester Datensatz (assembled exams + regs +
   constraints) + fester Seed → Diagnose-Report (Konfliktzahlen, Kosten) als
   Golden festhalten. Refactor darf ihn nicht verschlechtern. Da Stufe 1 dieselben
   Startzeiten nutzt, ist ein *gleichwertiger* Plan erwartbar; Abweichungen sind
   ein Warnsignal.
3. **Property-Tests über generierte Pläne:**
   - kein Studierender hat zwei Prüfungen mit Lücke `< ExamGapMinutes`;
   - kein Raum ist doppelt belegt innerhalb `TimelagMin`;
   - keine Aufsicht überlappt (inkl. eigener Prüfung + Reisezeit).
4. **Round-trip Persistenz:** `PlanEntry`/`PlannedRoom`/`Invigilation` als BSON
   schreiben+lesen (vgl. bestehendes `plexams/dump_test.go`), damit `Starttime`
   (TZ Europe/Berlin) exakt erhalten bleibt.
5. **Export-Konsistenz:** ICS/ZPA/CSV/PDF nutzen jetzt alle `Starttime` — ein Test,
   dass externe und interne Prüfungen dieselbe Zeitquelle liefern (behebt die
   heutige `ExternalTime`-Inkonsistenz).

---

## 7. GUI-Änderungen (für den plexams.gui-Agent)

Alle `(day, time)`/`(day, slot)`-Parameter werden zu Zeitparametern; day/slot-
Anzeigen zu Datum/Uhrzeit. Konkret:

- **Terminplan-Ansicht:** Raster wird von „N feste Slots/Tag" zu einer Zeitachse.
  Slots sind nur noch Gitterlinien (erlaubte Startzeiten), keine Identitäten. Drag
  & Drop / Zuweisung schreibt eine **Startzeit**, nicht (Tag, Slot).
- **Mutationen:** `addExamToSlot`/`preAddExamToSlot` → eine `setExamTime(ancode,
  start)`; `setPreplanExamSlot(dayNumber,slotNumber)` → `setPreplanExamTime(start)`.
- **Nicht-Standard-Startzeit-Hinweis:** Beim Setzen einer Zeit, die keine erlaubte
  Anfangszeit ist, zeigt die GUI einen Hinweis („keine Standard-Anfangszeit") mit
  Übernehmen-trotzdem-Option. Das Backend akzeptiert jede Zeit; ein Feld/Flag
  (`isStandardStarttime` o.ä.) signalisiert der GUI die Abweichung. Keine harte
  Ablehnung.
- **Räume:** `roomsForSlot`, `plannedRoomsInSlot`, `roomsWithFreeSeatsForSlot`,
  `blockRoomForSlot(s)`/`unblockRoomForSlot(s)` → zeitbasiert (`start`/`from,until`);
  `SlotInput` entfällt. `PlannedRoom`/`BlockedRoom`/`UnplacedExam` zeigen Zeit statt
  day/slot.
- **Aufsichten:** `roomsWithInvigilationsForSlot`, `invigilator(room,day,time)`,
  `prePlanInvigilation(day,slot)`, `PrePlannedInvigilation` → zeitbasiert;
  `Invigilation.slot` → `start`.
- **Preplan-Übersicht:** `PreplanSlotNeed` ohne day/slot, nur `starttime`.
- **Semester-Config-Formular:** „Slots" umbenennen zu „Anfangszeiten"
  (`startTimes`); `mucDaiSlots` (Zahlenpaare) → Datum/Uhrzeit-Auswahl
  (`mucDaiAllowedTimes`); neue Parameter „Turnaround (Min)" (`timelagMin`),
  „Puffer zwischen Prüfungen" (`examGapMinutes`) und „zu nah ab (Min)"
  (`notTooCloseMinutes`); Prüfungstage aus `semesterConfig.days` (nicht selbst aus
  from/until berechnen).
- **Slots vollständig entfernen:** kein „Slot"/„Slot-Nummer" mehr in der GUI —
  auch nicht in Tooltips, Tabellenköpfen oder Validierungs-/Fehlermeldungen.
- **Diagnose-Anzeige:** `sameSlot/adjacent/maxExamsPerSlot/slotsUsed` →
  `overlaps/tooClose/maxExamsAt/starttimesUsed`.
- **Konfliktliste:** Labels `SAME_SLOT/ADJACENT` → `OVERLAP/TOO_CLOSE` (aus dem
  Zeitabstand abgeleitet), Anzeige mit realen Uhrzeiten.

---

## 8. Sequenzierung (Schritt für Schritt: erst Backend, dann GUI nachziehen)

Vorgehen: pro Schritt zuerst Backend umsetzen (build/test/lint grün), dann die GUI
nachziehen (plexams.gui-Agent-Instruktionen aus Abschnitt 7), erst danach der
nächste Backend-Schritt. Slots sollen am Ende **komplett aus der GUI** verschwinden
— auch aus Validierungs-/Fehlermeldungen.

1. **Config-Umbau (Backend-Schritt 1):** `Slots`→`StartTimes`,
   `MucDaiSlots`→`MucDaiAllowedTimes`, `TimelagMin`+`ExamGapMinutes`+
   `NotTooCloseMinutes` in SemesterConfig, Tagesliste aus Backend. Legacy-Migration
   (goslots/goDay0) entfernen. Placement bleibt in diesem Schritt noch slot-basiert
   (kleinstmöglicher, in sich geschlossener Schritt). → **GUI:** Config-Formular
   (StartTimes, MUC.DAI-Zeiten, Turnaround/Gap/zu-nah, Tagesliste vom Backend).
2. **Placement-Modell (Backend-Schritt 2):** `PlanEntry` auf `Starttime`,
   Solver-Kandidaten aus `StartTimes`, Rückschreiben auf `Starttime`; Slot-CLI
   löschen; `getSlotTime`/`getSlotForTime` entfernen; Exporte auf `Starttime`.
   Golden-Snapshot etablieren. → **GUI:** Terminplan zeitbasiert, `setExamTime` mit
   Nicht-Standard-Hinweis.
3. **Konfliktregel zeitbasiert (Backend-Schritt 3, DONE):** die *gemeldete*
   Konflikt-Klassifikation ist zeit-/dauer-/NTA-basiert über eine gemeinsame
   `conflictcalc.TimeProximity` (Overlap/TooClose/SameDay/NextDay): genutzt in der
   Konfliktliste (`ExamScheduleConflicts`) und im Validator (`validate.go`), jeweils
   mit der **pro-Studierenden**-Dauer (Basis bzw. dessen eigener NTA — nicht die
   globale `MaxDuration`). Schwellen aus SemesterConfig (`ExamGapMinutes`,
   `NotTooCloseMinutes`). **Bewusste Scope-Entscheidung:** die *getunte
   Solver-Kostenfunktion* (`examplan` closeness/farness/hardConf/NTA-overrun) und
   deren Diagnostik-Buckets bleiben **unverändert** — sie bestimmen die Plan-Qualität
   und sind auf dem festen Raster bereits korrekt; die reine Zeit-Umstellung der
   Solver-Kosten gehört in Stufe 2 (feinere Granularität), wo sie neu kalibriert
   werden muss. Golden = die bestehenden `examplan`-Tests bleiben unverändert grün.
   → **GUI:** Konfliktlabels `SAME_SLOT/ADJACENT` → `OVERLAP/TOO_CLOSE`.
4. **Räume zeitbasiert:** DB + `rooms*.go` + Schema/Resolver. Turnaround.
   → **GUI:** Raumansichten zeitbasiert.
5. **Aufsichten zeitbasiert:** `invig*` + Schema/Resolver. → **GUI:** Aufsichten.
6. **Aufräumen:** `rooms_for_slots.go`, `slotTime`-Closures, tote CLI,
   `Slot`/`SlotInput`/`ExamDay.Number`-Reste, letzte Slot-Vorkommen in der GUI.

Jeder Schritt: `go build ./... && go test ./... && golangci-lint-v2 run`, und für
Schema-Schritte vorher `go generate ./...`.

---

## 9. Entscheidungen

**Entschieden (2026-07):**
- **Startzeit-Raster:** Startzeiten sind **nur Solver-Kandidaten**; jede Zeit ist
  erlaubt. Beim manuellen Setzen einer Zeit, die keine Standard-Anfangszeit ist,
  zeigt die GUI einen **Hinweis** mit der Möglichkeit, die Zeit **trotzdem zu
  übernehmen** (Override). Backend: `setExamTime` akzeptiert jede Zeit; der
  Validator/Resolver liefert ein Flag `isStandardStarttime` (bzw. eine Warnung),
  das die GUI vor dem Speichern anzeigt. Keine harte Ablehnung.
- **Turnaround:** **global** starten (ein `TimelagMin` für alle Räume in Stufe 1);
  pro Raum/Gebäude optional in Stufe 2.

**Noch offen:**
- **„Zu nah"-Schwelle:** fester Default (z.B. 120 Min) oder pro Semester
  konfigurierbar? Tendenz: konfigurierbar, Default 120.
- **`ExamDay`-Konzept:** Braucht die GUI weiterhin eine Tagesliste (für Spalten)?
  Falls ja, als abgeleitete `days: [Time!]` behalten (aus `from..until` ohne
  Wochenenden), aber ohne `Number`.
