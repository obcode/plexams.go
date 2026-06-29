---
name: preplanning-seb-exahm
description: "Planned feature: SEB/EXaHM pre-planning — manual pseudo-exams to book Anny rooms early for next semester, later linked to ZPA ancodes."
metadata:
  node_type: memory
  type: project
  originSessionId: 6285039b-3933-4bb1-a8f3-24a7355c4a1d
---

New planning step "Vorplanung SEB/EXaHM" (decided 2026-06-25). Anny rooms for the
NEXT semester must be booked very early, before the ZPA exam list / Primuss data
exist. Examers are asked by email (already done outside plexams, answers via Jira);
plexams should capture the data, give a room-need + overlap overview, and later
link each pseudo-exam to the real ZPA ancode.

Confirmed decisions:
- **Lives in the NEXT semester's DB** (created via the Phase-3 createSemester flow);
  the planner (re)starts plexams pointed at that semester to work on it.
- **Studiengänge become a first-class GLOBAL entity** `StudyProgram` (shortname/Kürzel
  unique, name, optional degree, `category` fk07|mucdai|misc, active) in the global
  `plexams` DB (like permanent_non_invigilators). (Until now programs were only strings
  from primuss `exams_XX` collections / config.)
  Seed sources: `zpa.fk07programs` (category fk07) + `mucdaiprograms` = DE/GS/ID
  (category mucdai; note these have ancode offsets DE=30000/GS=80000/ID=120000) + misc
  = GN (category misc). NB: GN/misc is currently in NO config key — add a `miscprograms`
  config key (or hardcode ["GN"] in the seed; it then lives in the DB).
- **Scope:** capture + overview + later ZPA linking ONLY. NO request email (exists
  already), NO Anny write/booking (only a read-only Anny token → plexams just reports
  the numbers; booking stays manual in Anny).
- **Slotting:** pre-exams ARE assigned to slots/go-slots → room need + program-overlap
  warnings per slot.
- **GUI-only** (no CLI for this feature — deliberate deviation from the CLI-sync rule).

PROGRESS (2026-06-25, session 6285039b): Phase 1 (StudyProgram, commit dfbea30) and
Phase 2 (PreplanExam CRUD+slot, commit f8356fa) are DONE on `main`, each verified against
the server. NOTE: the entity is named **`PreplanExam`** (collection `preplan_exams`) —
the name `PreExam` was already taken by an unrelated type (zpaExam/constraints/planEntry).
Phase 1 & 2 user-confirmed "im GUI umgesetzt". Phase 3 (overview) DONE too (commit
2b9d317): query `preplanOverview` → per slot (+ null-slot bucket) EXaHM/SEB seatsNeeded,
greedy roomsSuggested from real EXaHM/SEB rooms (EXaHM sized by Seats, SEB by SebSeats),
seatsAvailable, and program-overlap conflicts per slot. Phase 4 (ZPA linking) DONE too
(commit ec885d3): query preplanExamAncodeSuggestions(id) (ranked by examer+module),
mutations connectPreplanExamToAncode(id,ancode) (rejects unknown/already-linked) and
disconnectPreplanExam(id).

Phase 5 (assignment generate+validate): query validatePreplanAssignment and mutation
generatePreplanAssignment(keepAssigned) → PreplanValidation{ok,assignedCount,
unassignedIDs,messages}. REWRITTEN 2026-06-28 (commit 8776ec9). The old greedy
(largest-first + hill-climb) stranded exams even when feasible (11/27 in Test26SS).
Now it is treated as **graph colouring with bin capacities** (same program ⇒ not same
slot; each booked Anny slot = colour with seat limit) and solved by **DSATUR
constructive + simulated-annealing repair** (solver lives in plexams/preplan_solve.go,
helper solvePreplan; SA only runs if DSATUR leaves something unplaced, ejects ≤2 less
important units to make room — mirrors invigplan in spirit but self-contained).
EXaHM + large SEB weighted to never be the dropped ones; same-program spread across
days. Candidate slots = ONLY MUC.DAI slots with our booked Anny rooms; capacity =
~90% of booked physical seats (preplanCapacityFactor=0.9); no booked rooms ⇒ assign
nothing. Rooms for pre-planning are Anny (T-Bau) ONLY for both EXaHM and SEB
(requestWith=ANNY), honouring per-exam allowedRooms. Tests in preplan_solve_test.go.

REFINED 2026-06-28 (commit b82c35e) after reading live Test26SS (Mongo on
mongodb://localhost:27013, reachable via the Go driver even though /dev/tcp says closed;
global rooms live in DB `plexams.rooms`, anny personalization names in `plexams.anny_config`
= [Oliver Braun, Michael Heinl]). KEY FINDING: "same study program never in same slot" as
a HARD rule is infeasible (program IF is in 16/27 exams; only 10 booked Anny slots → ≥6
unplaceable) AND must-place demand 1379 ≫ booked capacity 972 (10 slots; 5 Anny rooms
T3.015/016/017/023=30, T3.021=1 = 121 phys, 90%≈108/full slot). So: same-program-same-slot
is now a SOFT cost (spread across slots then days; different exams of one program MAY run
simultaneously in different Anny rooms; exams that MUST run together use the same-slot
constraint). Only HARD constraint = seat capacity. SA-repair ejects smallest occupants to
free capacity. Priority: all EXaHM + large SEB placed first (never dropped); SEB that fit a
single R-building lab (≤ largest non-Anny SEB room, here 16) are kept OUT of Anny and
flagged "im R-Bau planen". When demand > booked capacity the result reports the shortfall
("mehr Anny-Slots buchen"). To place all, the planner must book more Anny slots — no
algorithm fits 1379 into 972.

FURTHER REFINEMENTS 2026-06-28 (live Test26SS, 27 exams):
- Candidate slots = ALL regular slots with booked Anny rooms, NOT only MUC.DAI slots
  (commit 927a366). Anny is booked on the normal grid (08:30/10:30/12:30/14:30/16:30);
  restricting to MUC.DAI slots ignored e.g. 24.07 14:30 (full booking) → stranded SEB.
- MUC.DAI slots are RESERVED: exams with a MUC.DAI program (DE/GS/ID, from
  p.mucdaiProgramNames / StudyProgram category mucdai) may ONLY go in (booked) MUC.DAI
  slots; others anywhere (preplanUnit.allowedSlots). preplanCapacityFactor 0.9→1.0 (fill
  full, no reserve) — both commit 2705d7c.
- Same-program-same-slot is SOFT and DISTANCE-based (commit 039e752): proximityPenalty =
  full at same slot, less with slot distance same day, 0 across different days ("möglichst
  verschiedene Tage; sonst max Slot-Entfernung"). SA now runs ALWAYS (not only when
  unplaced) + a swap move, else DSATUR-greedy clustered same program (e.g. DC). Explicit
  symmetric "nicht gleichzeitig" pairs: PreplanExam.NotSameSlot + mutation
  setPreplanExamNotSameSlot(id,otherID,conflict) (same distance logic, weight
  preplanExplicitConflictWeight=1000 vs program 100; both << dropBase=10000 so placement
  always wins). INVERSE hint (commit 7aa1edc): PreplanExam.CanShareSlot + mutation
  setPreplanExamCanShareSlot(id,otherID,canShare) — for pairs that share a program but NOT
  the same students (e.g. two Wirtschaftsinformatik): exempts that pair from the
  program-spreading penalty (preplanUnit.compatible), so they may share a slot/be adjacent.
  Symmetric pair links share helper setPreplanPairLink. Algorithms in docs/algorithmen.md.
- Live read via Go mongo driver works (mongodb://localhost:27013) even though /dev/tcp
  reports the port closed; throwaway test in plexams pkg with PLEXAMS_LIVE/PLEXAMS_WRITE
  env guards, deleted after use.

CONSTRAINT CARRY-OVER on connect (commit 7278d98): when a pre-exam is linked to a ZPA
ancode, only constraints with a ZPA counterpart that pre-planning can set are carried:
SEB/EXaHM room kind (from ExamKind), allowedRooms (if set), and same-slot — but same-slot
only between members BOTH connected. `syncPreplanGroupZPAConstraints` re-syncs the whole
transitive same-slot group on every connect AND disconnect, so a pair completes
automatically once the last member is connected and is fully removed again on disconnect
(no orphaned ZPA constraints; disconnect also RmConstraints the freed ancode). Reuses the
already-symmetric ZPA sameSlot machinery (AddConstraints/addAncodeToSameSlotConstraints).
Pre-plan stored constraints slimmed to same-slot + allowedRooms (preplanConstraintsFromInput);
old eager preplanConstraintsToInput removed. notSameSlot/canShareSlot are pre-plan-only (no
ZPA pendant). New query `preplanSameSlotGroups` → groups (size>=2) with per-member
{connected, ancode} + `complete` flag, so the GUI shows pending (not-yet-connected) members.

FIXED → LOCKED plan entry / Terminplan contract (commits c19419a, 2cb8b73): connecting a
FIXED (isFixed) pre-exam writes a LOCKED PlanEntry (Locked=true) for the linked ancode in
its slot (db.AddExamToSlot); disconnect removes it (RemovePlanEntry). Non-fixed connected
exams get only constraints, NO plan entry. **Contract for the future Terminplan generator
(next big step, like preplan/invigilation SA): a locked PlanEntry == hard-fixed; the
generator keeps locked entries and optimizes the rest** (same fix/optimize split as
invigplan Problem.Fixed + preplan isFixed). "Echtes Vorplanen" of any generated exam =
a locked entry; lockExam/unlockExam is the generic pin/unpin. GUI shows "vorgeplant"
should key off the real plan entry / isFixed, NOT the tentative pre-plan slot (every
pre-exam has one). Documented in docs/algorithmen.md. [[planning-state-model]]

NEW per-exam **isFixed** flag (commit 8776ec9): PreplanExam.IsFixed + mutation
setPreplanExamFixed(id,fixed) (rejects fixing an unslotted exam). On
generatePreplanAssignment: fixed exams keep their slot (pre-occupy capacity), ALL
non-fixed exams are re-planned (keepAssigned additionally keeps currently-slotted
non-fixed exams). Could not verify against live Test26SS (no Mongo/Docker in the
sandbox; user must mongoexport preplan_exams/anny_bookings/rooms to /home/ubuntu/semester
for data-driven validation).

Phase 6 (booking-aware validation) DONE (commit 23160ed): the deferred Anny-per-slot
dimension is now in. annyBookedBySlot maps existing Anny bookings to slots (slot start
within booking window) and sums booked EXaHM/SEB seats from room flags. PreplanKindNeed
gained seatsBooked + roomsToBook (rooms not yet booked, greedy to cover the gap);
preplanOverview fills them. validatePreplanAssignment now distinguishes physical-capacity
overflow from a booking gap ("Slot x/y: EXaHM noch N Plätze zu buchen (gebucht B von K
nötig) — z. B. R1, R2"); ok only once everything is booked. Lets the planner book in
Anny step by step.

DECISION (2026-06-25): generalize the invigplan SA optimizer into a generic
items→slots assign engine LATER, when the Terminplan generator is built (two real
consumers drive the abstraction), not now. The pre-planning stays greedy. The
Terminplan = the big future feature (initial plan + hard/soft + SA, e.g. Anny-room
availability as a constraint). See [[cli-to-gui-migration]].

FEATURE COMPLETE — pre-planning phases 1-5 on `main`, GUI-only.

Data model: `PreplanExam` (collection `preplan_exams` in the semester DB): id, examKind
(EXaHM|SEB), examerID (Teacher) + examerName snapshot, module (string), programs
([]Kürzel), expectedStudents, duration?, plannedSlot{day,slot}? , ancode? (null until
connected), notes. Overview from EXaHM/SEB room flags (Room.Exahm/Seb, Seats/SebSeats;
a SEB exam fits a SEB OR EXaHM room). Connect later via examer+module match suggestions.

Assumptions (proceed unless corrected): A1 examKind exactly one of EXaHM/SEB; A2 report
room COUNT (from EXaHM/SEB rooms), not just seats; A3 StudyProgram = Kürzel+name+degree?;
A4 after connect the PreExam is informational only (does NOT auto-fill rooms_pre_planned).

Implementation order (each its own commit): 1) StudyProgram (global) 2) PreExam CRUD+slot
3) overview (needs+conflicts per slot) 4) ZPA connect. See [[cli-to-gui-migration]].
