---
name: program-code-degree-suffix
description: "Study-program Kürzel stay two-letter (IF, DC); only DC-M carries the degree, as ZPA names it. Primuss file codes (IF-B) are mapped to the master data by the import; ZPA groups by longest ZPA-code prefix. Reworked 2026-11-27, no history kept."
metadata:
  node_type: memory
  type: project
---

Since 2026-WS there is a Bachelor **DC** and a Master **DC** (the former DA was renamed).
ZPA names the study groups `DC`, `DC5` (Bachelor) and `DC-M`, `DC-M1` (Master). The
Primuss Sammellisten file names have always carried the degree (`…-IF-B-226.xlsx`,
`…-DC-B-…`, `…-DC-M-…`).

**Model (2026-11-27, replaces the "suffix every program" design of c9e0d37):**
- `study_program.shortname` stays two-letter (`IF`, `DC`, `DA`, …). Only where two
  programs share the letters does it carry the degree, and then the way ZPA spells it:
  `DC-M`. `zpaCode` is normally empty (= shortname).
- **ZPA → program** ([db/program_resolver.go]): a group belongs to the program whose
  ZPA code is the *longest* prefix (`DC-M1` → DC-M, `DC5` → DC). No more "realized
  programs of this semester" cross-check — old semesters need not resolve anymore
  (user decision: "wir brauchen keine Historie"). A ZPA primuss-ancode tag `DC` on an
  exam that only has `DC-M` is moved to DC-M (`extendedProgram`).
- **Primuss → program** ([plexams/primuss/programs.go] `resolveFileProgram`): exact
  shortname (`DC-M`) wins; otherwise the code without degree, if that program's
  `degree` fits or is empty. Every file is resolved before anything is replaced, so a
  missing program gives a German message instead of the raw FK violation
  `primuss_exam_program_fkey` (which is what the first DC-B upload hit).
  A DC-M file never lands in the Bachelor DC: degree mismatch is an error.
- **ZPA outbound**: `zpaCodeForProgram` (zpaCode or shortname) → `DC-M` is posted as
  `DC-M`. Unverified whether ZPA expects exactly that for student regs.
- `validateStudyProgram`: fk07 Kürzel/ZPA code `^[A-Z]{2,4}(-[BM])?$`; no forced suffix.
  `defaultZpaCode` only reads the `zpacodes.<shortname>` override.

**To do in production:** create the program `DC-M` (fk07, Master) before the DC-M
Sammellisten are uploaded; `degree` must be set on DC (Bachelor) for the mapping to
refuse a DC-M file.
