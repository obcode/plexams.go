---
name: invigilator-no-duty-carryover
description: "Future design for carrying \"no invigilation\" reasons (Präsident/Dekanin/Mutterschutz) across semesters."
metadata:
  node_type: memory
  type: project
  originSessionId: 6285039b-3933-4bb1-a8f3-24a7355c4a1d
---

When the semester config is fully moved into the DB/GUI, design new-semester
creation so that semester-specific "does no invigilation" cases (Präsident,
Dekanin, Mutterschutz, …) can be carried over into the new semester on purpose,
rather than silently vanishing. Options the user floated:
- always require a **reason** for every "keine Aufsicht" entry, and/or
- keep Präsident/Dekanin/Mutterschutz in the **global** `permanent_non_invigilators`
  collection but with a **"temporär" flag**, surfaced at new-semester setup for
  review/confirmation.

**Why:** the asymmetry of mistakes — *not* planning someone who is actually back
on duty (e.g. a former Dekanin) is far less harmful than planning someone who is
still excused. So a carry-over/review step should err on the side of keeping the
exclusion until explicitly cleared.

Current state (2026-SS): only the 6 retired ("pensioniert") people are in the
global `permanent_non_invigilators` collection; Präsident (245), Dekanin (301),
Mutterschutz (7313) and "vorgearbeitet" (246) stay per-semester in
`invigilator_constraints`. See [[cli-to-gui-migration]].
