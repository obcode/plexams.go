# How a semester of exam planning runs (FK07)

This document is for planners and developers who want the whole run of a semester
on a few pages. plexams is driven entirely through the GUI
([`plexams.gui`](https://github.com/obcode/plexams.gui)); the backend keeps all data
in PostgreSQL. There is no CLI and no YAML file to edit along the way.

**The leading source is the GUI itself:**

- The **start page** shows every step as a checklist in five phases. Clicking a step
  opens the page where it is done.
- The **help page `/hilfe`** („Werkzeuge" → „Hilfe") explains every step (what, why,
  pitfalls), the validation levels, the error messages and the glossary. It is more
  detailed than this file and is generated from the same step list; when the two
  disagree, `/hilfe` wins.

The step list is defined in `plexams/planning_state.go`; the explanations shown in the
GUI live in `plexams.gui/src/lib/help/workflow.js`.

**Dry run first.** Every step that writes data or sends mail has a dry run:
„Probelauf" on the generator pages computes and shows the result without saving;
on „E-Mails" → „E-Mails versenden" every send is a „Probelauf" by default (the mails
go only to the planner's test address), real sending is „Wirklich senden …" with a
confirmation. ZPA uploads of exams also offer a dry run. Always look at the dry run
before the real run.

**Ticks.** Most checklist ticks are set automatically when the operation succeeds
(marked „auto" below). A few are set by hand on the start page. „Alle Prüfungen
anderer FKs terminiert" is computed and cannot be set by hand.

Before the first step, a semester has to exist: „Semester" → „Neues Semester
anlegen", then „Semester-Konfiguration" (exam period, start times, mail settings).
Rooms, study programmes and NTA master data are kept across semesters under
„Semester" → „Stammdaten (für alle Semester)".

---

## Phase −1: still in the previous semester

Computer exams (EXaHM and SEB) need the special rooms in the T-Bau, which have to be
booked long in advance: ask for demand, book rooms, spread the computer exams over the
booked times.

| Checklist step | Page | What to do / what it writes | Tick |
|---|---|---|---|
| Personen aus ZPA importiert | „Vorbereitung" → „ZPA-Import & Veröffentlichung" | „Aus dem ZPA laden" for the teachers (Dozierende). Overwrites the local mirror of the ZPA teachers. | auto |
| EXaHM/SEB-Abfrage verschickt | „E-Mails" → „E-Mails versenden" | Send the query who plans an EXaHM or SEB exam. Send-once mail. | auto |
| EXaHM/SEB-Prüfungsbedarf erfasst | „Vorbereitung" → „Abfragen an Prüfende" | Review and apply the examiners' answers; enter answers that came in another way by hand in the Vorplanung. „Schließen" on the inquiry sets the tick, reopening it clears it. | auto |
| Anny-Räume gebucht | „Vorbereitung" → „Anny-Buchungen" | Book the T-Bau rooms in Anny itself (not in plexams), usually with 30 minutes before and after, 60 minutes for 120-minute EXaHM exams. Then fetch the bookings on this page; they define the times at which EXaHM/SEB exams can take place. | by hand |
| SEB/EXaHM-Vorplanung erzeugt | „Vorbereitung" → „SEB/EXaHM-Vorplanung" | Distribute the reported computer exams over the booked times, by hand or with „Automatisch verteilen" (re-places every pre-planned exam not fixed with 🔒). Shows early whether the bookings suffice. | by hand |

## Phase 0: Vorbereitung

The data comes together: exams from the ZPA, registrations from Primuss, the examiners'
wishes. Only after linking them does plexams know which exams share students.

| Checklist step | Page | What to do / what it writes | Tick |
|---|---|---|---|
| Prüfungen aus ZPA importiert | „Vorbereitung" → „ZPA-Import & Veröffentlichung" | „Aus dem ZPA laden" for the exams. ZPA data is read-only in plexams; errors are fixed in the ZPA and reloaded. | auto |
| EXaHM/SEB-Vorplanung mit ZPA-Prüfungen verknüpft | „Vorbereitung" → „SEB/EXaHM-Vorplanung" | Assign each pre-planned computer exam to its ZPA exam (ancode). Only then does the real exam take over the pre-planned time. | by hand |
| Prüfungen gemeinsamer Studiengänge importiert & verknüpft | „Vorbereitung" → „Prüfungen gemeinsamer Studiengänge" | „CSV importieren" for the other faculties' exams (e.g. MUC.DAI) and link them to the ZPA exams. Deleting the imported exams clears the tick. | auto |
| ZPA-Prüfungen für die Planung ausgewählt | „Vorbereitung" → „Prüfungen auswählen & Vorgaben" | Decide per ZPA exam whether we plan it (oral exams, Scheine etc. are not). | by hand |
| Prüfungsplanungs-Info an Prüfende verschickt | „E-Mails" → „E-Mails versenden" | Tells examiners which of their exams are planned, so they can report constraints. The send to all goes out once; single examiners can still be mailed. | auto, when sent to all |
| Primuss-Anmeldedaten importiert | „Vorbereitung" → „Prüfungslisten aus Primuss" | „Sammellisten-ZIP hochladen" with the lists of all programmes (delivered by the Prüfungsamt after the registration deadline). „Sammellisten zurücksetzen" clears the tick. | auto |
| ZPA- & Primuss-Prüfungen verknüpft | „Vorbereitung" → „Zuordnung ZPA ↔ Primuss" | Check that every exam to plan found its Primuss registrations; fix missing or wrong links (ancodes differ especially in joint programmes). | by hand |
| Vorgaben der Prüfungen eingetragen | „Vorbereitung" → „Prüfungen auswählen & Vorgaben" | Enter the constraints per exam: fixed time, excluded days, „gleichzeitig mit", room requirements. The generator and the validation only know what is entered here. | by hand |
| Aufbereitete Prüfungen erstellt | „Vorbereitung" → „Aufbereitete Prüfungen" | „Generieren" merges ZPA data, Primuss registrations, NTAs and constraints per exam. Only possible once the four steps ZPA import, Primuss import, ZPA ↔ Primuss link and constraints are ticked. Replaces the previous assembled exams; „Zurücksetzen" deletes them (times, rooms and constraints stay) and clears the tick. Regenerate after every change of registrations, NTAs or constraints; a banner on the page says when they are stale. | auto |
| Anmeldungen je Studierender:m erstellt | same page | Created in the same „Generieren" run: one list of registrations per student, the basis of the conflict check. | auto |
| Anmeldungen ins ZPA hochgeladen | „Vorbereitung" → „ZPA-Import & Veröffentlichung" | „Ins ZPA übertragen" → „Anmeldungen". Students then see their exams in the ZPA. No dry run. Upload again when registrations change. | auto |
| Primuss-Daten an alle verschickt | „E-Mails" → „E-Mails versenden" | Every examiner gets the registrations for their exam. Send-once as a bulk mail; the mail for a single exam can be resent any time. | auto |
| Info an NTAs mit eigenem Raum verschickt | „E-Mails" → „E-Mails versenden" | Informs students with an NTA entitling them to a room of their own (and their examiners). Send-once. | auto |

## Phase 1: Terminplanung

Every exam gets a day and a start time. The generator does most of the work; the planner
checks the result, judges the remaining conflicts and corrects by hand.

| Checklist step | Page | What to do / what it writes | Tick |
|---|---|---|---|
| Alle Prüfungen anderer FKs terminiert | „Termine" → „Prüfungen anderer FKs" | Enter the times the other faculties fixed for their exams. They are fixed for us; the generator plans around them. | computed: set as soon as all such exams have a time, cleared when one is missing |
| EXaHM/SEB in T-Bau-Räume geplant | „Termine" → „EXaHM/SEB in T-Bau planen" | „Probelauf", then „Planen & speichern". Overwrites the times of all EXaHM/SEB exams except those with a fixed time from the Vorplanung; other exams stay untouched. | auto |
| EXaHM/SEB fixiert (für Phase 2) | same page | „Fixieren": fixed computer exams keep their time when the full schedule is generated. „Fixierung aufheben" clears the tick. | auto |
| Terminplan generiert | „Termine" → „Terminplan generieren" | „Probelauf", read the report (unplaced exams, conflicts), then „Generieren & speichern" with the same seed. Overwrites the times of all exams except those with a fixed time from the Vorplanung, the fixed EXaHM/SEB exams and those of other faculties; an exam that finds no slot loses its old time. Nothing is saved if hard rules are violated. „Generierten Terminplan zurücksetzen" removes the generated times and clears the tick. Then correct by hand on „Prüfungsplan" and check on „Terminplanung validieren". | auto |
| Entwurf verschickt 🔒 | „E-Mails" → „E-Mails versenden" | Send the draft to the examiners. Send-once. From now on regenerating the schedule is locked; changes by hand remain possible. | auto |
| Terminplan veröffentlicht (E-Mail) 🔒 | „Vorbereitung" → „ZPA-Import & Veröffentlichung", then „E-Mails" | After agreement, „Ins ZPA übertragen" → „Prüfungen" (dry run first), then send the publication mail. „Veröffentlicht" means the mail is sent; the ZPA upload is a separate step and always allowed. Send-once. | auto |

## Phase 2: Raumplanung

Every scheduled exam gets rooms, every student a seat. Rooms that do not belong to the
faculty are requested from the Gebäudemanagement first.

| Checklist step | Page | What to do / what it writes | Tick |
|---|---|---|---|
| Raum-Anfragen ans Gebäudemanagement verschickt | „Räume" → „Raum-Anfragen (Gebäudemanagement)" | Load the preview (dry run, changes nothing), „Übernehmen" (creates the requests and **replaces all existing ones**, entered approvals are lost — the page asks first), then „E-Mail senden". Enter approvals per request under „Bestehende Anfragen" as they arrive. Send-once; „Übernehmen" is locked once the room plan is published. | auto |
| Räume zugeordnet | „Räume" → „Räume generieren" | „Probelauf", then „Generieren & speichern" (optionally starting from the existing assignment). Rooms belong to the exam: if an exam is moved afterwards, generate rooms again. „Raumzuteilung zurücksetzen" clears the tick. Block rooms per slot and fix rooms on „Räume" → „Raumplanung". | auto |
| Raumbelegung ans Sekretariat verschickt | „E-Mails" → „E-Mails versenden" | Tells the secretariat which rooms are used when. Send-once. | auto |
| Raumplan veröffentlicht (E-Mail) 🔒 | „Vorbereitung" → „ZPA-Import & Veröffentlichung", then „E-Mails" | „Ins ZPA übertragen" → „Prüfungen mit Räumen", then send the publication mail. From now on regenerating rooms is locked; single rooms can still be changed by hand. Send-once. | auto |
| EXaHM/SEB-Raumübersicht ans KDP verschickt | „E-Mails" → „E-Mails versenden" | Room overview of the computer exams for the KDP. Send-once. | auto |

## Phase 3: Aufsichtenplanung

Every room and exam time gets an invigilator, plus reserve invigilators. Before that the
invigilators state when they are unavailable.

| Checklist step | Page | What to do / what it writes | Tick |
|---|---|---|---|
| Aufsichts-Anforderungsabfrage verschickt | ZPA, then „E-Mails" → „E-Mails versenden" | Open the requirements form in the ZPA, then send the request mail. Send-once. | auto |
| Aufsichts-Anforderungen importiert | „Vorbereitung" → „ZPA-Import & Veröffentlichung" | After the deadline „Aus dem ZPA laden" → „Aufsichts-Anforderungen". Whoever entered nothing gets a reminder from „E-Mails" (repeatable); load again afterwards. | auto |
| Aufsichten eingeteilt | „Aufsichten" → „Aufsichten einteilen" | Run as „Probelauf (speichert nichts)", then choose „speichern (ersetzt die Einteilung)" and „Einteilung starten". Replaces the whole previous assignment; pre-planned invigilations stay where they are. Respects excluded days, own exams and „Aufsichten" → „Aufsichts-Vorgaben". Then „Aufsichten validieren". | auto |
| Aufsichtenplan veröffentlicht (E-Mail) 🔒 | „Vorbereitung" → „ZPA-Import & Veröffentlichung", then „E-Mails" | „Ins ZPA übertragen" → „Prüfungen mit Räumen & Aufsichten", then send the publication mail (every invigilator gets the personal plan). From now on re-assigning is locked; single invigilations can still be changed by hand. Send-once. | auto |
| Info 'Aufsichten veröffentlicht' ans Sekretariat verschickt | „E-Mails" → „E-Mails versenden" | Send-once. | auto |
| Info an NTAs zu ihren Räumen verschickt | „E-Mails" → „E-Mails versenden" | Students with an NTA learn in which room they write which exam. Send-once. | auto |
| Wiederholungsprüfungen der Lehrbeauftragten an die LBA-Beauftragten verschickt | „E-Mails" → „E-Mails versenden" | Overview of the repeat exams of Lehrbeauftragte with their times and invigilations. Send-once. | auto |
| Deckblätter an alle verschickt (letzter Schritt) | „E-Mails" → „E-Mails versenden" | Every examiner gets the cover pages of their exams. The last step of the planning. Send-once; a single cover page can be resent. | auto |

---

## Validation

Validation checks the current state against the planning rules and changes nothing.
Run it after every larger step:

- „Termine" → „Terminplanung validieren", „Räume" → „Räume validieren",
  „Aufsichten" → „Aufsichten validieren",
- „Werkzeuge" → „Validierung (alle Bereiche)",
- or by clicking the validation light in the top bar.

While a validation runs, changes are briefly locked. Levels:

| Level | Meaning | What to do |
|---|---|---|
| ⛔ Fehler (error) | A rule is violated; the plan cannot stay like this. | Fix before publishing. |
| ⚠️ Warnung (warning) | Unfavourable or still open, may be intended. | Look at each and decide. |
| • Info | A note, not a problem, e.g. a conflict that was accepted. | Nothing. |
| – übersprungen (skipped) | The check cannot run yet because its data is missing (e.g. no schedule, no rooms, no invigilations yet). | Nothing; it runs once the step before is done. |

A check passes (✓) when it has no errors, even with warnings left. The light in the top
bar is red on errors, yellow on warnings, green otherwise.

## Locks

- **Published plans.** „Entwurf verschickt" and „Terminplan veröffentlicht" lock
  generating the schedule (including resetting it); „Raumplan veröffentlicht" locks
  generating rooms and applying room requests; „Aufsichtenplan veröffentlicht" locks
  generating invigilations. These steps carry 🔒 on the checklist. Explicit changes by
  hand and ZPA uploads stay allowed. For a new generator run, untick the step on the
  start page, regenerate, and tick it again.
- **Send-once mails.** While a mail's step is ticked, a real send is refused; the dry run
  still works. To send again, untick the step on the start page.
- **Protected semester.** A semester can be set to read-only via the semester selector
  in the top bar („Dieses Semester schützen"); every change is then refused until
  „Schutz aufheben". A red banner shows the state.
- **Running operations.** While a validation, an import, a ZPA transfer, a generator run
  or a mail send is running, other changes are refused; wait and retry.
- **Roles.** Accounts with the role VIEWER can only read; planning needs PLANER.
