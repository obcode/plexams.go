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

**Status 2026-07-15 (verified against repo):** the carryover *mechanic* now EXISTS.
- Global collection `permanent_non_invigilators` (`db/permanent_non_invigilators.go`,
  `db/collection.go`), model `PermanentNonInvigilator{TeacherID, Name, Reason}`
  (`graph/model/permanent_non_invigilator.go`), business layer
  `plexams/invigilator_constraints.go` (`PermanentNonInvigilators`,
  `SetPermanentNonInvigilator`, `RemovePermanentNonInvigilator`, `notInvigilating`),
  GraphQL resolvers in `graph/invigilation.resolvers.go`.
- `notInvigilating` unions the global permanent set with per-semester
  `invigilator_constraints.IsNotInvigilator`, so a global entry carries across semesters.
- Commits `4089c79`, `f9166bf` (display name), `821c245`, `da7dfaa`, `f91c701`.

**Still open (the refinement this note was about):**
- No **"temporär" flag** — the model is a single flat permanent list; it can't
  distinguish role-bound/temporary exclusions (Präsident/Dekanin/Mutterschutz, should
  expire) from truly permanent ones (retired).
- `Reason` is **not required** — `SetPermanentNonInvigilator` accepts an empty reason;
  no enum/validation.
- No **new-semester review/confirmation step** surfacing carried-over exclusions.

See [[cli-to-gui-migration]].
