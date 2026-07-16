---
name: mucdai-to-joint-generalization
description: "MUC.DAI special-casing generalized to first-class \"joint study programs\" so MUC.HEALTH (+future) is just data."
metadata:
  node_type: memory
  type: project
  originSessionId: 90f09a57-0245-428e-bf4f-c4afbf033f3d
---

Generalized the hardwired **MUC.DAI** mechanism into first-class **joint study
programs** (gemeinsame Studiengänge) so MUC.HEALTH (FK07+FK11, one shared program)
and any future joint Studienfakultät are just data. Built 2026-07-16 (uncommitted
working tree at time of writing; build/vet/lint/test all green). Decisions taken with
Oliver: keep a named `jointFaculty` label ("MUC.DAI"|"MUC.HEALTH") AND go per-program;
full rename of the API surface `MucDai*`→`Joint*`; term = "joint". FK10 (`notPlannedByMe`
free-text constraint) is orthogonal and untouched.

**Model changes:**
- `StudyProgram.Category "mucdai"` → **`"joint"`**; new field `JointFaculty *string`
  (bson `jointFaculty`). Startup migration `migrateMucdaiToJoint` (plexams/study_programs.go,
  called from newPlexams) flips legacy category+sets JointFaculty="MUC.DAI" (idempotent,
  ~3 global docs). Predicate helper `mucdaiProgramNames`→**`jointProgramNames`** (filters
  category=="joint"). Config seed: new `jointfaculties: [{name, programs}]` list (viper
  UnmarshalKey; `mucdaiprograms` still read as MUC.DAI fallback); `externalExamsBase.<prog>`
  unchanged. `jointFacultyConfigsFromViper` helper.
- **Per-program reserved slots** replace the single global `MucDaiAllowedTimes`/`MucDaiSlots`.
  New `model.JointProgramTimes{Program, AllowedTimes []time.Time}` (bson, hand model in
  semester_config_input.go) + GraphQL types `JointProgramTimes`/`JointProgramTimesInput`/
  `JointProgramSlots{program, slots}`. `SemesterConfigInput.JointProgramAllowedTimes
  []*JointProgramTimes`; derived `SemesterConfig.JointProgramAllowedTimes` + `JointProgramSlots`.
  `deriveMucDaiSlots`→`deriveJointProgramSlots(slots, jpts)` (pure, unit-tested in
  joint_slots_test.go). LEGACY field `MucDaiAllowedTimes` kept (bson `mucDaiAllowedTimes`,
  json:"-") ONLY for backward-compat; `applyLegacyJointTimes` (loadSemesterConfig) expands it
  to ALL joint programs when the per-program field is empty (transitional).

**Reservation semantics (the real logic):** an exam is restricted to the **intersection**
of the reserved slots of every joint program its students belong to (was: any MUC.DAI
student ⇒ the one MUC.DAI slot set). A joint program with NO reserved times imposes no
restriction (preserves prior single-faculty behavior). Two sites:
- examplan_build.go (~L440): builds `progSlotIdx map[prog][]slotIdx` from
  `sc.JointProgramSlots`; per unit intersect (sorted progs for determinism);
  empty→unplaceable `unplaceableNoJointSlot`.
- preplan_assign.go (~L433): `progSlotIdx map[prog]map[int]bool`; per unit intersect;
  nil=unrestricted, empty non-nil=unplaceable via existing `intersectSlotSet`.
- validate_db.go `validStarttimes()` unions regular + ALL joint programs' slots.

**Full rename (mucdai→joint), gqlgen regenerated:**
- files: db/mucdai*.go→db/joint*.go, plexams/mucdai*.go→plexams/joint*.go, plexams/mucdai/
  pkg→plexams/joint/, graph/mucdai.*→graph/joint.*.
- GraphQL: `importMucDaiExams`→`importJointExams`, `setMucDaiZpaLink`→`setJointZpaLink`,
  `removeMucDaiLink`→`removeJointLink`, `mucDaiZpaCandidates`→`jointZpaCandidates`,
  `mucdaiExams`→`jointExams`; types `MucDaiExam`→`JointExam`, `ImportMucDaiResult`→
  `ImportJointResult`. `setExternalExamTime` kept generic.
- collections: `mucdai_<prog>`→`joint_<prog>`, `mucdai_links`→`joint_links` (rebuilt on
  import; NO auto-migration → manual joint links created pre-upgrade are redone on re-import).
  dump dataset key `mucdai-links`→`joint-links`.
- consts: `mucDaiPlannerFK07`→`jointPlannerFK07` (value still "FK07"=us); link-kind consts
  `jointLink*`; `condMucDaiImported`→`condJointImported` but **stored VALUE kept
  "mucDaiImported"** (don't orphan existing planning state).
- KEPT as-is (faculty-specific report, no rename): `draftMucDaiMaroto` / PDF kind
  "draft-muc.dai" (hardcoded DE/ID/GS, sibling of draftFk08/draftFk10).

**Rollout (in-place, additive):** deploy→migration flips 3 programs; register MUC.HEALTH
program(s) via GUI (category=joint, jointFaculty, externalExamsBase); planner enters
per-program reserved times per semester (legacy fallback bridges); re-import each faculty's
CSV via `importJointExams`.

**GUI-sync needed** (plexams.gui-agent): StudyProgram form `jointFaculty` + category
mucdai→joint; semester-config form per-program reserved-times editor
(`jointProgramAllowedTimes`, read back `jointProgramSlots`) replacing the single MUC.DAI
input; rename `MucDaiExam`→`JointExam`/`mucdaiExams`→`jointExams`/`importMucDaiExams`→
`importJointExams`/`setMucDaiZpaLink`/`removeMucDaiLink`/`mucDaiZpaCandidates`→Joint*;
group joint exams by `jointFaculty`; planning-state label renamed. Live E2E (per-faculty
reserved windows honored + links resolve) still pending real Mongo data.

[[mucdai-import-linking]] [[studentreg-dual-ancodes]] [[preplanning-seb-exahm]]
[[slotless-timebased-redesign]] [[gui-and-cli-sync]]
