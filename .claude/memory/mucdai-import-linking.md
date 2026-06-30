---
name: mucdai-import-linking
description: MUC.DAI import builds explicit stored links of each MUC.DAI exam to an external or ZPA exam.
metadata:
  type: project
---

MUC.DAI import (built 2026-06-30, commits 7e1365d, 75a44a7, 172eee3) — planning-state
point `mucDaiImported` (phase0), marked after `ImportMucDaiExams`.

At import time, `relinkMucDaiExams` (re)builds an explicit per-semester collection
**`mucdai_links`** (key program+primussAncode): non-FK07 MUC.DAI exams link to their
auto-created external (non-ZPA) exam; **FK07** exams link to the ZPA exam whose
`primussAncodes` contain (program, primussAncode) exactly (unique → `linked`, else
`unresolved` — 0/-1 placeholder, missing, or ambiguous). The resolved ancode is stored
EXPLICITLY so a later ZPA re-import can't silently break it. Manual links
(`source=manual`) are preserved; stale links dropped. Auto = linked (no approve step).
Pure decision helper `autoMucDaiLink` (unit-tested in mucdai_links_test.go).

`model.MucDaiExam.linkStatus` = "external" | "zpa" | "unresolved" (enrichMucDaiExams now
reads `mucdai_links`, not the live connected-exams derivation). GUI resolve/correct:
query `mucDaiZpaCandidates(program, primussAncode)` (ranked ZPA suggestions) + mutations
`setMucDaiZpaLink(program, primussAncode, zpaAncode)` / `removeMucDaiLink(program,
primussAncode)`. ZPA primussAncodes DO carry DE/GS/ID entries; many with ancode 0 = the
missing-number cases. [[preplanning-seb-exahm]]
