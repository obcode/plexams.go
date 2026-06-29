---
name: zpa-import-behaviors
description: "ZPA exam import auto-presets the to-plan status, and stale banners only appear after the first generation."
metadata:
  node_type: memory
  type: project
  originSessionId: 6285039b-3933-4bb1-a8f3-24a7355c4a1d
---

Two deliberate behaviors around `importExamsFromZPA` (decided 2026-06-29, commits 195a529 + eff6078):

1. **Auto-preselect to-plan status.** After the ZPA exam import, `autoPreselectExamsToPlan`
   (in plexams/zpa_import.go, called at the end of `ImportExamsFromZPA`) sets the planning
   status of every exam that has NO decision yet: written + practical exams → to plan, all
   others → not to plan. Rule = `examShouldBePlanned`: `ExamTypeFull` (bson `examtypefull`)
   contains "schriftliche prüfung" or "praktische prüfung" (case-insensitive) — matches the
   constraints-email wording. **Manual decisions are preserved** (re-import only classifies
   newly-added undecided exams). In Test26SS: 324 → 124 to-plan, 200 not-to-plan, 0 unknown.
   Lives in the shared Plexams method, so CLI (`zpa exams`) and GUI both get it. The planner
   can still move individual exams between to-plan / not-to-plan.

2. **No "generated exams / student regs stale" banner before the first generation.** The
   caches can only be stale once generated at least once. `MarkGeneratedExamsDirty` /
   `MarkStudentRegsDirty` AND the state getters (`GeneratedExamsState`/`StudentRegsState`)
   now report not-dirty while the cache is empty (CountGeneratedExams / CountStudentRegsPlanned
   == 0). Fixes the premature "veraltet" banners right after the first ZPA import (before any
   Primuss data, when nothing can be generated). [[planning-state-model]]
