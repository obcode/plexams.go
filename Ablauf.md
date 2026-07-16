# Ablauf der Prüfungsplanung (FK07)

Dieser Ablauf beschreibt die Prüfungsplanung mit `plexams.go` (Server/CLI) und
`plexams.gui` (Web-Interface) auf dem **aktuellen Stand** nach der Umstellung
vieler Schritte von der Kommandozeile auf die GraphQL-API / GUI.

Grundsätzliches zur aktuellen Architektur:

- **Vieles läuft jetzt über das GUI** (GraphQL). Lang laufende Operationen
  (Generierungen, ZPA-Transfers, E-Mail-Versand) streamen ihre Ausgabe zeilenweise
  ins GUI. Die entsprechenden CLI-Befehle existieren weiterhin als Alternative.
- **E-Mail-Versand**: im GUI als Subscription mit einem `run`-Schalter. `run: false`
  ist ein Probelauf, der nur an die Testadresse (`smtp.testmail`) geht; `run: true`
  verschickt echt. Auf der CLI entspricht das dem `-r`-Flag.
- **Räume, Raum-Requests, NTAs, Raum-Blocks** liegen in der MongoDB und werden über
  das GUI gepflegt — **nicht mehr über `roomConstraints` in der `<Semester>.yaml`**.
- Pro Semester eine eigene MongoDB-Datenbank; Räume und NTAs in der globalen DB
  `plexams`.

> Hinweis: Befehle in `code` sind CLI-Befehle. Steht „(GUI)" dabei, ist das der
> empfohlene Weg über `plexams.gui`; der CLI-Befehl ist die Alternative.

---

## Zustandsmodell: Phasen, Meilensteine, Gates

Der Ablauf wird als **Bedingungs-/Ereignis-Petrinetz** abgebildet (1-sicher):
jede Phase hat eine Reihe von **Bedingungen** (Meilensteinen). Eine Bedingung kann

- **automatisch** gesetzt werden, wenn eine Operation erfolgreich durchläuft
  (z.B. „Räume für Prüfungen generiert", „Terminplan veröffentlicht"), oder
- **von Hand** an-/abgehakt werden.

Das Modell ist im Code definiert (`plexams/planning_state.go`) und dort
erweiterbar; der Zustand liegt pro Semester in der DB (`planning_state`). Im GUI
wird es auf der **Startseite** als Checkliste pro Phase angezeigt
(GraphQL: Query `planningState`, Mutation `setPlanningCondition`).

**Gates (Sperren).** Manche Bedingungen sperren, solange sie gesetzt sind, die
zugehörigen **Generierungen** (Inhibitor-Kante):

| Bedingung (gesetzt = veröffentlicht) | sperrt |
|---|---|
| `roomPlanPublished` | `generateRoomsForSlots`/`-Exams`, `applyRoomRequestsPreview` |
| `invigilationPlanPublished` | `generateInvigilations` |

„Veröffentlicht" heißt: die **Veröffentlichungs-E-Mail** wurde verschickt — *nicht*
der ZPA-Upload. Immer erlaubt bleiben: **ZPA-Upload**, **Anny-Import** und alle
**expliziten Änderungen** (Raum blocken/freigeben, vor-/entplanen, `change-room`,
Requests genehmigen/deaktivieren, `move-to`). Kleine Korrekturen nach der
Veröffentlichung: explizit ändern und erneut ins ZPA laden (ohne neue Sammel-Mail),
oder das Häkchen kurz zurücksetzen, neu generieren, wieder setzen.

**Einmalige E-Mails.** Die meisten Workflow-E-Mails dürfen nur **einmal** verschickt
werden. Ist die zugehörige Bedingung gesetzt, ist ein echter Versand (`run: true`)
gesperrt — nur der **Probelauf** (`run: false`) geht noch. Zum erneuten Versenden
das Häkchen zurücksetzen. Betroffen sind: EXaHM-Abfrage (`exahmRequested`),
Constraints (`constraintsRequested`), „zu planende Prüfungen" (`examsPrepared`),
Draft (`draftSent`), Terminplan/Räume/Aufsichten veröffentlicht
(`examPlanPublished`/`roomPlanPublished`/`invigilationPlanPublished`),
Raum-Anfragen (`roomRequestsSent`), Aufsichts-Anforderungsabfrage
(`invigilationsRequested`), **Primuss-Daten an alle** (`primussDataAllSent`),
**Info an NTAs mit eigenem Raum** (`ntaRoomAloneSent`, nach der Ankündigung „zu
planende Prüfungen"), **Info an NTAs zu ihren Räumen** (`ntaPlannedSent`, direkt
vor den Deckblättern) und — als **letzter Schritt der Aufsichtenplanung** —
**Deckblätter an alle** (`coverPagesSent`).

**Nicht** einmalig (wiederholbar, jederzeit erneut sendbar): Primuss-Daten für
**eine** Prüfung und für nicht von uns geplante Prüfungen, „neuer NTA", die Info an
einen **einzelnen** NTA mit eigenem Raum, das einzelne Deckblatt und die Erinnerung
an fehlende Anforderungen. Alle E-Mails sind im GUI verfügbar.

---

## Phase -1: noch im vorherigen Semester

- **Abfrage per E-Mail, wer SEB oder EXaHM plant** — `email exahm` (Probelauf),
  `-r` für echten Versand (GUI: `sendEmailExaHM`).
- **Räume im T-Bau über [anny.eu](https://anny.eu) buchen**, gut über den
  Prüfungszeitraum verteilen.
  - Prüfungsslots beginnen immer um 08:30, 10:30, 12:30, 14:30, 16:30 (, 18:30).
  - Buchungen in Anny immer +30 Minuten vor- und nachher, d.h. bei 08:30 Start und
    90 Minuten Prüfung: Buchung 08:00–10:30.
  - Auf Ausnahmen achten: z.B. eine EXaHM-Prüfung mit 120 Minuten braucht 60 Minuten
    Vor-/Nachlauf, also bei 08:30 Start: Buchung 07:30–11:30.
  - **Neu:** Die Anny-Buchungen werden **direkt aus Anny in die DB importiert** —
    kein ICS-/`bookings.yaml`-/`roomConstraints`-Umweg mehr:
    - „Anny-Buchungen importieren" (GUI: `importAnnyBookings`) bzw. `rooms anny`.
    - Daraus ergeben sich die zulässigen Slots der EXaHM-/T-Bau-Räume.
- **Vorplanung EXaHM-/SEB-Prüfungen** (im Numbers-Dokument).
- **Festlegung Prüfungszeitraum** in den letzten PK-Sitzungen (Vorgabe durch
  Prüfungsausschuss / Satzung):
  - WiSe: Prüfungen ab 26.01.; SoSe: Prüfungen ab 11.07.
  - Prüfungen an FK07: immer nur Mo–Fr, 10 Tage.

---

## Phase 0: Vorbereitung

Zu Beginn des Semesters, sobald die Frist der PKVs für die Prüfungsmodalitäten
abgelaufen ist und <https://zpa.cs.hm.edu/public/exam_means_list/> funktioniert:

- **JIRA-Tickets für Absprachen öffnen**
  - mit Prüfungsplaner FK10 (Prüfungszeitraum),
  - mit Prüfungsplanern MUC.DAI (FK03, FK08, (FK12)) (Tag 0),
  - mit MUC.DAI-Verantwortlichen und Prüfungsplanern (Abfrage nach MUC.DAI-CSVs).
- **Prüfungen und Personen aus dem ZPA einlesen** — `zpa exams`, `zpa teacher`
  (GUI-Import bzw. `make fetch-zpa`).
- **Abfrage per E-Mail bzgl. Besonderheiten** — `email constraints -r`
  (GUI: `sendEmailConstraints`, `run: true`).
- **Nach Rückmeldung:**
  - Auswahl der zu planenden Prüfungen (GUI → ZPA → Prüfungsliste).
  - Einpflegen der Constraints (GUI).
- **MUC.DAI-Prüfungs-CSVs** der anderen FKs einpflegen → Makefile.
- **Durch FK10 geplante Prüfungen** einpflegen → Excel von PKV → GUI.
- **Vorplanung EXaHM/SEB einpflegen** — `plan pre-plan-exam ...`.
- **Ancodes MUC.DAI** überprüfen/korrigieren (GUI → Vorbereitung → zu planende
  ZPA-Prüfungen).
- **Non-ZPA-Prüfungen** der anderen FKs erzeugen — `prepare add-mucdai-exams`
  (Fakultätspräfixe wie `30` für FK03 werden automatisch hinzugefügt).
- **Veröffentlichung „zu planende Prüfungen" und „Constraints"** in Confluence
  inkl. E-Mail-Benachrichtigung.

---

## Phase 1: Terminplanung

Nachdem die Sammellisten aller Studiengänge vom Prüfungsamt da sind.

- **Primuss-Daten einlesen**
  - Sammellisten in Ordnerstruktur speichern, idealerweise `make all` (sonst
    `make csvs` + `make mongoimports`; unter macOS `brew install gnu-sed`).
  - Übersichten im GUI unter Primuss → Prüfungslisten.
- **Wahlpflichtfächer MUC.DAI identifizieren und Ancodes ergänzen** (GUI →
  Vorbereitung → „Anmeldungszuordnung (ZPA/Primuss)").
  - Anmeldecodes nur mappen, wenn die Meldungen zusammenpassen.
  - Fehlende Ancodes ergänzen mit `primuss add-ancode` (+ Doku im Makefile).
- **MUC.DAI-Prüfungen der anderen FKs einpflegen** — `prepare add-mucdai-exams`.
- **ConnectedExams erstellen** — `prepare connected-exams`
  (zu planende ZPA-Prüfungen mit den Primuss-Prüfungen verknüpfen). Im GUI unter
  Vorbereitung → Anmeldezuordnung kontrollieren (besonders MUC.DAI).
- **GeneratedExams erstellen** — `prepare generated-exams`
  (Prüfungsobjekte aus ZPA-Prüfung + Primussdaten + NTAs + Constraints).
- **StudentRegs erstellen** — `prepare studentregs`
  (pro Studierendem ein Objekt mit allen Anmeldungen, Basis der Konfliktprüfung).
- **StudentRegs ins ZPA hochladen** — `zpa studentregs` (Studierende sehen die
  Prüfungen dann im Kalender). Vorher E-Mails holen: `zpa students`.
- **Überprüfen, ob NTA-Bescheide noch gültig sind.**
- **E-Mails mit den Primuss-Daten verschicken** — `email primuss-data all`
  (Probelauf ohne `-r`); einzeln per Ancode `email primuss-data <ancode>`; für
  nicht von uns geplante Prüfungen `email primuss-data-unplanned <prog> <ancode> <mail>`.
- **E-Mails an NTAs mit Anspruch auf eigenen Raum** — `email nta-with-room-alone`.

**Bei Änderungen** (Primuss-Daten, neue/​geänderte NTAs, Constraints):

- GeneratedExams und StudentRegs neu erstellen.
- Nur bei geänderten Anmeldungen: StudentRegs erneut ins ZPA hochladen + betroffene
  Prüfende informieren.
- NTAs pflegen: bestehende im GUI (NTA-Verwaltung) bearbeiten, neue im GUI anlegen
  (Matrikelnummer/E-Mail ggf. via `info student -z <Nachname>`). Danach studentregs
  und generated-exams neu generieren, dann `email new-nta <mtknr> -r`.

**Jetzt planen:**

- Planen über CLI (`plan move-to <ancode> <day> <slot>`), nicht Drag&Drop.
- **Validierung mitlaufen lassen** — `validate conflicts` / `validate constraints`
  (CLI: `make validate-exams-planning`; GUI: die `validate*`-Subscriptions).
  - Reservierte Zeiten der gemeinsamen Studiengänge (MUC.DAI/MUC.HEALTH) visuell prüfen.
  - Akzeptable Konflikte in `<Semester>.yaml` unter `knownConflicts`.
  - **Semester-Zeiten** (`validateSemesterTimes`): prüft die Startzeiten gegen das
    Tageszeit-Fenster (Winter: nicht zu früh; Sommer: nicht zu spät). Eigene Prüfungen
    außerhalb → Fehler (HARD) bzw. Warnung (SOFT); abweichende EXaHM/SEB-Einplanungen
    (klimatisierter T-Bau) nur als Info.
- EXaHM/SEB-Vorplanung gegen Anmeldungen/Konflikte prüfen. „Prüfung planen" im GUI
  zeigt geeignete Slots farblich an.
- Prüfungen einzeln verplanen (Best Practice: nach Studiengängen verteilen, mit den
  größten Prüfungen beginnen).

**Nach abgeschlossener Terminplanung:**

- Vorläufigen Plan ins ZPA pushen, Draft-PDFs/-CSVs erstellen.
- E-Mail an Prüfende (`email prepared`/`email draft`), E-Mail an Fachschaftsleitung.
- SpecialInterest-PDFs erstellen und verteilen; Drafts verteilen (MUC.DAI, KDP).

**Nach Abstimmung:**

- Plan ins ZPA pushen — `zpa upload-plan` (GUI: `uploadExamsWithRoomsToZpa`).
- E-Mail an Prüfende und FS — `email published-exams` (GUI: `sendEmailPublishedExams`).
- Aufsichtenabfrage im ZPA freischalten, E-Mail an Aufsichten (siehe Phase 3).

---

## Phase 2: Raumplanung

- **Info-E-Mails an alle Studierenden mit NTA und Anspruch auf eigenen Raum**
  (Prüfende im CC), spätestens jetzt.
- **Raum-Anfragen ans Gebäudemanagement** (Roter Würfel R1.006/R1.046, Blaue Tonne
  R1.049 usw.) — **neu komplett im GUI / über die DB**, nicht mehr per YAML:
  1. Probelauf ansehen (GUI: `roomRequestsPreview`, read-only) bzw.
     `info request-rooms`.
  2. Übernehmen (GUI: `applyRoomRequestsPreview`) — schreibt die Requests in die DB
     (ersetzt vorhandene; Schutz gegen versehentliches Überschreiben genehmigter
     Requests).
  3. Einzelne Requests genehmigen / deaktivieren, bei Bedarf manuell hinzufügen oder
     zeitlich verlängern (z.B. wegen NTA).
  4. Anfrage-E-Mail ans Gebäudemanagement (`ueberlassung-gm@hm.edu`,
     konfiguriert über `semesterConfig.emails.roomManagement`) — GUI:
     `sendEmailRoomRequests`, CLI: `email room-requests`. Die Zeiten enthalten
     ±15 Min Vor-/Nachlauf.
  - Unterscheidung: T-Bau-Räume werden über **Anny** angefragt, die übrigen über das
    **Gebäudemanagement** (`Room.requestWith` = ANNY/MANAGEMENT/NONE,
    `requestPriority` steuert die Reihenfolge bei der Generierung).
- **Räume blockieren** (statt des früheren `roomConstraints.*.notAvailable`):
  Ein Raum, der in einem Slot anderweitig belegt ist, kann pro Slot (auch mehrere
  Slots / ganzer Tag auf einmal) blockiert werden (GUI: `blockRoomForSlot(s)` /
  `unblockRoomForSlot(s)`, CLI: `plan block-room` / `plan unblock-room`). Blockierte
  Räume werden bei der Generierung nicht verwendet.
- **Räume für Prüfungen generieren** — GUI: `generateRoomsForExams`
  (CLI: `prepare rooms-for-exams`).
  - **Neu:** Der frühere separate Schritt `rooms-for-slots` ist nicht mehr nötig —
    die zulässigen Räume pro Slot werden bei `rooms-for-exams` automatisch zuerst neu
    berechnet. `generateRoomsForSlots` (CLI: `prepare rooms-for-slots`) gibt es weiter
    als optionalen Vorschau-Schritt.
  - Achtung: Räume hängen an der Prüfung, nicht am Slot. Wird eine Prüfung danach
    verschoben, hat sie noch die alten Räume → neu generieren.
  - Vorgeplante Räume werden bei der Generierung übernommen (ein in seinem Slot
    blockierter vorgeplanter Raum wird mit Warnung übersprungen).
  - Im GUI unter Plan → Raumplanung: Räume aus der Vorplanung sind markiert; generierte
    Räume können per „OK" in die Vorplanung übernommen (= fixiert) werden — wirksam
    nach erneutem `generateRoomsForExams`.
  - Soll ein anderer als der generierte Raum genutzt werden → `plan pre-plan-room ...`
    (Entfernen: `plan rm-pre-plan-room ...`) und neu generieren.
- **NTAs ohne Anspruch auf eigenen Raum**: möglichst mit in einen normalen Raum
  (dann ist dieser Raum im Folgeslot ggf. nicht nutzbar → abwägen).
- **NTAs mit Anspruch auf eigenen Raum**: werden in R3.01x eingeplant; SEB in ein
  eigenes Labor; EXaHM/SEB-Einzelraum im T-Bau über Anny buchen.
- **Bei Absage einzelner Prüfungen**: Anmeldung mit `primuss rm-studentreg ...`
  entfernen, danach generated-exams und studentregs neu generieren (Ancode =
  Primuss-Ancode, ≠ ZPA-Ancode bei MUC.DAI).
- **Bei EXaHM-/SEB-Prüfungen**: prüfen, ob NTA-Studierende in die gebuchten Räume
  passen (Anspruch/Dauer), Dauern prüfen, nicht benötigte Räume in Anny freigeben.
- **Nach Umplanung über Anny**: Buchungen neu holen (GUI: `importAnnyBookings`,
  CLI: `rooms anny`); danach Räume neu generieren.
- **Raumplanung überprüfen**:
  - Validierung — GUI: `validateRooms*`-Subscriptions, CLI: `validate rooms`. Dazu
    gehören u.a.: Räume pro Slot/Exam, Request nötig, Zeitabstände, **blockierte
    Räume** (ein blockierter, aber verplanter Raum ist ein Fehler) und **veralteter
    rooms-for-slots-Cache** (Hinweis, neu zu generieren).
  - Sichtprüfung im GUI → Plan → Raumplanung. Räume per „OK" fixieren.

---

## Phase 3: Aufsichtenplanung

Wenn der Terminplan veröffentlicht ist.

- **Anforderungen an die Aufsichten einholen**
  - Im ZPA freischalten: <https://zpa.cs.hm.edu/regulations/publish_exam_plan/>.
  - E-Mail-Aufforderung — `email invigilations` (GUI: `sendEmailInvigilations`),
    eine Woche Frist.
  - Anforderungen holen — `zpa invigs` (GUI: `importInvigilatorRequirementsFromZpa`);
    fehlende werden im GUI angezeigt (auf Freisemester/Pensionierung prüfen).
  - Nach Fristablauf Erinnerung — `email invigilations-missing`.
  - Wenn alles da ist, die ZPA-Möglichkeit wieder ausschalten und letzten Stand noch
    einmal fetchen.
- **Zusätzliche Anforderungen** (z.B. längere Abwesenheit) kommen über Jira und werden
  in `<Semester>.yaml` übernommen (wirksam beim Start / per Config-Reload; gelöschte
  Einträge erfordern Neustart).
- **Aufsichten vorplanen** — `invigilation -p ...` (fix bei der Generierung).
- **Todos generieren** — `prepare invigilator-todos`.
- **Eigenaufsichten generieren** — `prepare self-invigilations` (eigene Prüfungen in
  nur einem Raum, an nicht ausgeschlossenen Tagen).
- **Constraints**: Hard (ausgeschlossene Tage, Mindestabstand, max. 1 Aufsicht/Slot),
  Soft (max. 3 Anwesenheitstage, ±60 Min zur Soll-Zeit).
- **Aufsichten automatisch generieren** — GUI: `generateInvigilations` (mit `dryRun`),
  CLI: `invigilation generate --dry-run --iterations 2000000`.
  - Greedy-Startplan; vorgeplante Aufsichten und Eigenaufsichten sind fix.
  - Optimierung mit Simulated Annealing (Fixed + Hard Constraints immer eingehalten,
    Soft Constraints über Kostenfunktion).
  - Probelauf (`dryRun`/`--dry-run`) speichert nichts. Sieht der Plan gut aus, ohne
    Dry-Run generieren (ersetzt den alten Plan); sonst mehr Iterationen / anderer Seed.
- **Validieren** — `validate invigilator-reqs invigilator-slots` (GUI:
  entsprechende `validate*`-Subscriptions).
- **Veröffentlichen** — individuelle E-Mails an die Aufsichten mit persönlichem Plan
  (PNG + ICS) — GUI: `sendEmailPublishedInvigilations`.

---

## Benötigte Software

- MongoDB (z.B. im Docker-Container).
- Server und CLI: <https://github.com/obcode/plexams.go>.
- Web-Interface: <https://github.com/obcode/plexams.gui>.
- Optional für direkten GraphQL-Zugriff: z.B. <https://www.usebruno.com>.
- Für den Primuss-Excel-Import: `ssconvert` (Paket `gnumeric`) und `mongoimport`.

Daten über alle NTAs und die Räume müssen bereits in der MongoDB vorhanden sein.
