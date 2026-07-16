---
name: program-code-degree-suffix
description: "Study-program Kürzel may now be degree-suffixed (DC-B/DC-M); StudyProgram.zpaCode maps back to the 2-letter ZPA code at the boundaries. Backend DONE (build/test/lint green), no old-semester migration, GUI-sync pending."
metadata:
  node_type: memory
  type: project
  originSessionId: 77b62a2f-c900-4edf-88a5-3d4376bdabc6
---

Next semester there are both a Bachelor **DC** and a Master **DC**, breaking the old
"2-letter Studiengangskürzel is unique" assumption. Fix (uniform, user-chosen): every
fk07 program gets a degree suffix internally (`IF-B`, `DC-B`, `DC-M`, …); **ZPA keeps
its 2-letter codes** and a translation layer bridges them.

**Decisions:** suffix ALL programs (not just DC); **no migration of old semesters**
(they stay 2-letter, archival); ZPA may later adopt unique codes → the mapping is
transitional and degenerates to identity when `ZpaCode == Shortname`.

**Design — mapping lives on the global `StudyProgram` entity:** new field
`ZpaCode string` (empty ⇒ identity = Shortname). Boundaries translated:
- **ZPA inbound** ([db/zpa_exams.go] `programResolver` + `cleanupPrimussAncodes`):
  replaced `group[:2]` with a resolver that maps ZPA code → internal suffixed program.
  It's **semester-safe**: cross-checks master `ZpaCode` candidates against the programs
  actually realized (`GetPrograms`) this semester, so old `exams_IF` semesters still
  resolve to `IF`, new `exams_IF-B` resolve to `IF-B`. Ambiguous dual codes (DC→DC-B/DC-M)
  fall back to the raw code + a warning; the isolated `degreeSuffixedForGroup` hook is
  the TODO extension point once real ZPA group names for dual codes are known.
- **Primuss inbound** ([plexams/primuss/import.go] `primussGroupRE`): now
  `-([A-Z]{2,4}-[BM])-` keeps the degree, so `…-DC-B-…xlsx`/`…-DC-M-…xlsx` land in
  separate `studentregs_DC-B`/`_DC-M` collections (they collided in `_DC` before).
- **ZPA outbound** ([plexams/zpa_post.go] + `zpaCodeForProgram`): student-reg upload
  translates internal suffixed program → 2-letter `ZpaCode` so ZPA still receives `DC`.
- Enumeration regexes widened: `GetPrograms` / `studentRegsCollectionNames` →
  `^exams_[A-Z]{2,4}(-[BM])?$` etc. Collection naming (`getCollection` fmt.Sprintf)
  already accepts hyphens; ICS route `{program}` (chi) too.

Seed derives `ZpaCode` by stripping `-B`/`-M` (`DC-B`→`DC`); optional YAML override
`zpacodes.<shortname>: <code>`. Config/docs updated ([docs/configuration.md]).

**Status:** backend DONE & **on main** (merge c9e0d37, 4 commits), `go build/vet/test/lint`
green; unit tests for resolver, regex, filename parsing, defaultZpaCode. **NOT run:** Mongo
integration (no mongod in this env) — importing a DC-B/DC-M ZIP + round-trip upload.
**GUI-sync pending:** `StudyProgram` editor needs `zpaCode` (+ `degree`) fields; program
pickers/ICS/CSV must pass suffixed codes. Builds on [[mucdai-to-joint-generalization]]
and [[studentreg-dual-ancodes]]; supersedes the 2-letter assumption noted in
[[mucdai-import-linking]].
