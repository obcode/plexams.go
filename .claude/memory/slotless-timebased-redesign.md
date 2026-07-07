---
name: slotless-timebased-redesign
description: "Planned big refactor — drop slot/day numbers entirely, store exams as absolute times; conflicts/rooms/invigilation become interval-based."
metadata:
  node_type: memory
  type: project
  originSessionId: 70930680-8219-4f0b-b5a8-65a5b770bd06
---

Decided direction (planner Oliver, 2026-07): replace the ordinal
`(DayNumber, SlotNumber)` placement model with **absolute times only**. Motivation:
changing the exam period today shifts the meaning of every stored day/slot number.

Key decisions:
- **Full slot elimination**, not the conservative "keep slots internal" variant.
- **No data migration** — clean cut starting with a new semester.
- **CLI is being removed** ([[cli-to-gui-migration]]) → delete slot-related CLI
  parts (cmd/plan.go etc.) instead of adapting them.
- Conflicts: time-distance rule (overlap / gap < ExamGapMinutes / "too close")
  instead of sameSlot/adjacent — more correct (duration + NTA aware).
- Rooms & invigilation: interval + turnaround buffer (`GenerationConfig.TimelagMin`
  already exists for invigilations) instead of slot buckets + "next slot blocked".
- Config `Slots []string` reinterpreted as "allowed start times"; `MucDaiSlots
  [][]int` → absolute times.

Staging (representation ≠ behaviour): **Stufe 1** = time representation at the SAME
granularity as today's slots (golden-testable "same input → equivalent plan");
**Stufe 2** (later) = finer/free start times + room capacity in the solver.

Goal this serves: summer semester should fit all exams into mornings (see
[[slot-time-avoidance]]) — finer time granularity gives the solver more room.

Detailed file-by-file plan + GUI change list: `docs/plan-slotless-timebased.md`
in the repo. Remember [[gui-and-cli-sync]] — needs plexams.gui-agent instructions.

Progress (backend, step-by-step then GUI catches up):
- **Step 1 DONE** (on main): config renamed Slots→StartTimes, MucDaiSlots→
  MucDaiAllowedTimes ([]time.Time), TimelagMin+ExamGapMinutes+NotTooCloseMinutes
  in SemesterConfig, legacy goslots/goDay0 migration removed.
- **Step 2 DONE** (branch/committed): PlanEntry now persists absolute `Starttime`
  as the source of truth; DayNumber/SlotNumber are `bson:"-"` derived on read via a
  db-injected SlotResolver (plexams implements SlotForTime/TimeForSlot, set in
  deriveSemesterConfig). ExternalTime field removed → `External bool` flag (foreign)
  + Starttime (time). New `setExamTime` GraphQL mutation. cmd/plan.go deleted.
  Tests: plexams SlotForTime round-trip + db decoration (verified vs real mongod via
  downloaded standalone mongod, see [[mongotest-without-docker]]).
- **Step 3 DONE** (on main): conflicts classified by TIME/DURATION/NTA via shared
  conflictcalc.TimeProximity (OVERLAP/TOO_CLOSE/SAME_DAY/NEXT_DAY), used in the
  conflict list (ExamScheduleConflicts) and the ValidateConflicts scan, each with the
  PER-STUDENT duration (their own NTA, not the exam's global MaxDuration — planner's
  correction). Thresholds ExamGapMinutes/NotTooCloseMinutes from SemesterConfig.
  DELIBERATELY did NOT touch the tuned solver cost (examplan closeness/hardConf) —
  that drives plan quality and is correct on the grid; pure-time solver cost is
  stage 2. Existing examplan tests = "plans unchanged" golden. GraphQL proximity
  values SAME_SLOT/ADJACENT → OVERLAP/TOO_CLOSE (GUI must map).
- **Step 4 DONE** (on main): PlannedRoom/UnplacedExam/BlockedRoom persist Starttime
  (source of truth), derive Day/Slot on read via the same db SlotResolver decoration.
  BlockedRoom moved to a hand-written model (graph/model/blocked_room.go). Starttime
  stamped centrally in PrepareRoomForExams (per-slot). Blocks keyed room+starttime.
  GraphQL room types gained `starttime`; (day,time) signatures kept (translate
  internally). Room turnaround (ValidateRoomsTimeDistance) + per-slot assignment
  UNCHANGED (already grid-correct — planner confirmed the turnaround they wanted
  already existed interval-based). db room-decoration test vs real mongod.
- **Step 5 DONE** (on main): Invigilation + PrePlannedInvigilation are hand-written
  models persisting Starttime (truth); Invigilation.Slot / PrePlanned Day/Slot derived
  on read via db decoration. In-slot filters + pre-planned keys resolve via starttime.
  Fixed the zero-Starttime bug in db.AddInvigilation. invigplan solver unchanged
  (already time-native). GraphQL PrePlannedInvigilation gained starttime.

**Stufe 1 COMPLETE** (steps 1–5 on main). All persisted planning data (config, exam
placement, rooms, blocked rooms, unplaced, invigilations, pre-planned invigilations) is
now absolute-time-based with day/slot derived on read; conflicts are time/duration/NTA
based. No data migration (clean cut, new semesters). GUI must be caught up per the
per-step instructions. **Stufe 2 (in progress):** solver cost pure-time + finer/free start-time granularity +
capacity; eventual full removal of day/slot from GraphQL/GUI.
- **A1 DONE** (on main, feat(examplan)): the solver's HARD student constraint is now
  time-overlap based (Problem.overlaps + per-pair hardSep = occ incl NTA + gap), replacing
  same-slot veto + NTA-overrun-next-slot (removed nextSlot/prevSlot/overrun*/ntaAdjOK/
  SetNTAOverruns). Grid-equivalent (existing examplan tests unchanged = golden). This
  unblocks finer/shorter StartTimes producing CORRECT (overlap-free) plans — the planner
  can experiment now by editing StartTimes.
- **Room window DONE** (on main, feat(rooms)): booked rooms (anny/GM) must be available
  [start-15, start+BASE+15]; a single NTA runs into the 15-min post-buffer (larger NTAs →
  separate rooms later, not a scheduling showstopper). GM request window uses base+buffer
  (dropped examMaxDuration); anny CoversSlot → Covers(from,until,winStart,winEnd), offered
  only if the booking covers [start-15, start+block+15]. (block=slot spacing as exam-length
  proxy on the grid; per-exam-duration precision is a finer-grid follow-up.)
- **Room turnaround DONE** (on main, feat(rooms)): ValidateRoomsTimeDistance rewritten
  to a true interval check per room over ALL its uses (PlannedRoom.Starttime + Duration,
  sorted, consecutive gap ≥ TimelagMin), replacing the consecutive-grid-slots loop (+ its
  buggy len(Days)-1 guard). Granularity-independent → shortening start times stays correct
  on the room side. Remaining finer-grid caveat: anny coverage still uses the slot block
  as exam-length proxy (needs per-exam duration when granularity < exam length).
- **D (de-slot API) part 1+2 DONE** (on main): removed day/slot OUTPUT fields from
  PlanEntry/PlannedRoom/UnplacedExam/BlockedRoom/PrePlannedInvigilation (all expose
  starttime); converted plan/room/invigilation in-slot QUERIES + block/pre-plan MUTATIONS
  to `starttime: Time!` (resolvers convert via SlotForTime); renamed examsInSlot→examsAt,
  plannedRoomsInSlot→plannedRoomsAt, roomsForSlot→roomsAt, roomsWithFreeSeatsForSlot→
  roomsWithFreeSeatsAt, roomsWithInvigilationsForSlot→roomsWithInvigilationsAt,
  blockRoomForSlot(s)→blockRoomAt/blockRoomAtTimes, prePlanInvigilationInSlot→
  prePlanInvigilationAt; invigilatorsForDay/invigilator take date/starttime; SlotInput
  removed; RoomsForSlot gains starttime. Added Plexams.DayNumberForDate. Internal
  (day,slot) grid unchanged (model structs keep derived Day/Slot). GUI must switch these.
- **D COMPLETE** (on main): remainder done — preplan (setPreplanExamTime,
  PreplanExam.plannedStarttime, PreplanSlotNeed de-slotted), room_request (RoomRequest/
  Preview expose starttime, mutations take starttime; both models hand-written to keep
  Day/Slot as the db key), ValidationFinding.day/slot→starttime (central conversion in
  validation.add; newValidation takes p.TimeForSlot), Slot type → {starttime} (model.Slot
  hand-written; internal DayNumber/SlotNumber kept). ExamDay.number/Starttime.number remain
  as grid ordinals. GraphQL API fully time-based; day/slot only inside Go. Timezone verified:
  backend emits Berlin local offsets (never UTC Z).
- **A2a DONE** (branch feature/slotless-stufe2, refactor(examplan)): the SOFT cost logic is
  now Start-derived, not slot-ordinal. NewProblem precomputes dayOfSlot + slotDayPos from each
  slot's absolute Start (calendar-day grouping + chronological position); closeness/farness,
  the CrossCampus same-day test, the interior-hole grouping and the diagnostics bucket use
  dayOfSlot/slotDayPos instead of SlotRef.Day/.Slot. Grid-equivalent by construction (a day's
  chronological positions reproduce SlotNumber±1), so examplan golden tests are UNCHANGED.
  SlotRef.Day/.Slot fields still exist (build/writeback use them until slice B) but drive no
  cost. This makes the objective correct at any start-time granularity.
- **C DONE** (branch, feat(examplan)): configurable per-time seat cap. New
  SemesterConfig(Input).MaxSeatsPerSlot (0/unset = unlimited) plumbed through GraphQL
  (SemesterConfig + SemesterConfigInput + SemesterConfigInputData input), the hand input
  model (bson round-trips), the derived-config builder + regenerated gqlgen. buildExamPlanProblem
  sets each examplan.Slot.Seats = MaxSeatsPerSlot; feasible()/canSwap() already enforce it. On
  the grid = per-slot concurrent load; with finer times = the per-time cap that keeps overlaps
  roomable WITHOUT wiring the room inventory into the solver (the option Oliver chose:
  "konfigurierbares Limit pro Zeit"). GUI: add maxSeatsPerSlot to the Semester-Config form.
- **Writeback + SlotRef DONE** (branch, 2 more commits): the generator writes placements via
  SetExamTime(start) (behaviour-identical to old AddExamToSlot force=true) and the conflict
  slot-model is keyed by Starttime. Then SlotRef lost its Day/Slot fields ENTIRELY — the
  examplan package now carries NO day/slot ordinals: the room-overrun same-day test uses a
  calendar-day compare (sameCalendarDay in the build), violation Refs use Problem.slotDayRef
  (derives a readable (day,pos) from dayOfSlot/slotDayPos), dead TopLoadedSlots removed, all
  test SlotRef literals are Start-only. examplan golden tests green (grid-equivalent). The
  plexams build GLUE still forms [2]int{day,slot} keys from model.Slot for the SHARED
  annyBookedBySlot helper (6 callers incl. preplan) — that signature de-keying is deliberately
  left as a separate low-value step.
- Remaining Stufe 2 (branch feature/slotless-stufe2): **B** finer/free start times just need
  the planner to add StartTimes to config — candidates already flow through per-(day,slot); the
  cleanup is de-keying buildExamPlanProblem/AllowedSlots/MucDaiSlots + writeback to time and
  removing SlotRef.Day/.Slot; **A2b** OPTIONAL further recalibration of closeness to a
  continuous time-gap falloff (needs real Test26SS data — neighbor-based is already
  granularity-safe); **D** remove day/slot from remaining GraphQL (Slot type in slot1/slot2 +
  Invigilation.slot, ExamDay.number, diagnostics rename sameSlot→overlaps) + internal model
  fields + GUI slot labels; clean rooms_for_slots.go remnants.
