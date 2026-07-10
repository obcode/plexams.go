---
name: studentreg-dual-ancodes
description: "StudentRegs now carry Primuss AND ZPA ancode explicitly; internal=ZPA, external(MUC.DAI/Primuss)=Primuss; + Ancodes value type."
metadata:
  node_type: memory
  type: project
  originSessionId: c44d9555-93c0-4a9a-9715-36609a7f3003
---

Built 2026-07-10, **merged to `main` and pushed** (commits 30524ab refactor + 7fbbee7 csv).
Makes the two ancode namespaces explicit on student registrations.

**Problem:** `PrepareStudentRegs` used to OVERWRITE `StudentReg.AnCode` in-place Primuss→ZPA
(prepare.go, "fixing ancode"), so the planned view (`Student.Regs`, `RegWithProgram.Reg`) kept
only ZPA and lost the Primuss ancode. Also many `ancode` fields were namespace-ambiguous, and
`GetStudentRegsForAncode` used the ZPA ancode as a Primuss key → empty regs for MUC.DAI exams.

**Rule:** internal (plan/slot/conflict) → **ZPA ancode**; external comm (Primuss/MUC.DAI) →
**Primuss ancode** (per-program, only unique with Studiengang, e.g. DE/202). FK07 usually equal
but NOT assumed (Prüfungsamt errors) — the connected-exam `PrimussAncodes` mapping is authoritative.

**Core new type** `model.Ancodes{ ZpaAncode int; PrimussAncodes []ZPAPrimussAncodes{Program,Ancode} }`
= exactly `ZPAExam.AnCode`+`ZPAExam.PrimussAncodes` as a named bundle (graph/model/zpa.go).
Helpers: `ZPAExam.Ancodes()`, `ZPAExam.PrimussAncodeForProgram(prog)`, and
`(*AssembledExam|*PlannedExam).Ancodes()` (computed, no stored field — GraphQL `ancodes: Ancodes!`
binds to the method). Consolidated the duplicate `plexams.PrimussAncode` map-key struct into
`model.ZPAPrimussAncodes`.

**Changes:**
- `RegWithProgram`: `reg` → `primussAncode` + `zpaAncode` (per-program projection of Ancodes).
- `Student.regs` → `zpaAncodes`. `StudentReg.ancode`/`EnhancedStudentReg.ancode` → `primussAncode`
  (Go field `StudentReg.PrimussAncode`, **bson stays `"AnCode"`** for import). `AnCode.ancode`
  (zpaAnCodes query) → `zpaAncode`. `StudentRegsPerAncodeAndProgram` → dual `zpaAncode`+`primussAncode`.
- Pure helper `resolveAncodes(program, primussAncode, primussToZpa) (primuss, zpa)` in
  plexams/studentreg_ancodes.go (unit-tested); prepare.go stops overwriting, sets both.
- Bug fix `GetStudentRegsForAncode`: resolve Primuss ancode per program from `zpaExam.PrimussAncodes`
  (iterate those not Groups; skip ancode<=0) before the raw reg lookup.
- Tier-3 fixes: `ValidateConflicts` knownConflicts display no longer uses the broken `ancode%1000`
  decode (which assumed the never-called `AddMucDaiExamByProgram` base+primuss scheme; import uses
  90000-sequential) — now renders `exam.ZpaExam.PrimussAncodes[0]` as `program/primuss (ZPA: n)`.
  CSV `exam-times` gained read-only `program`+`primussAncode` columns (import still keys on `ancode`).
- Draft PDF `draft-muc.dai` was already correct (shows Primuss ancode).

Left as-is: dead types `StudentRegsPerAncode`/`StudentRegsPerStudent` (latter still used by
`NTAWithRegs.regs`). Optional follow-up: rename Primuss-only exam types (`PrimussExam.ancode`,
`Conflict(s).ancode`) → primussAncode; close `assembledExams.go` `// TODO: add external exams`.

**GUI-sync needed** (plexams.gui-agent): `RegWithProgram.reg`→`zpaAncode`; `Student.regs`→
`zpaAncodes`; `StudentReg/EnhancedStudentReg.ancode`→`primussAncode`; `AnCode.ancode`→`zpaAncode`;
`StudentRegsPerAncodeAndProgram` new `zpaAncode`+`primussAncode`; new `Ancodes` type + `ancodes`
field on AssembledExam/PlannedExam. After deploy: re-run "Prüfungsanmeldungen aufbereiten" (derived
collection, no migration). [[mucdai-import-linking]] [[gui-and-cli-sync]]
