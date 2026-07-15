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

**DROPPED (Oliver 2026-07-15):** the earlier refinement plan — a "temporär" flag +
mandatory reason + a new-semester review/confirm step — is **not** being built. In
practice Oliver just puts the temporary cases (with a reason) into the surviving global
`permanent_non_invigilators` collection and **deletes** them again when they no longer apply.

**New direction Oliver floated (not built, TBD):** deleting is not ideal — it loses
history and can retroactively corrupt past semesters (if invigilation is recomputed live
against the *current* global collection, a deleted exclusion makes the person look like
they were available back then). Instead, model it **like the NTAs**: a `Deactivated`
flag (see `plexams/nta.go` `SetNTAActive` → `db.SetNtaDeactivated`, model field
`Deactivated bool` + server-managed `LastSemester`). A deactivated entry is **kept**
(history preserved) but **not applied** to future prepare/generate — so revisiting an old
semester still plans it historically correctly. Open question: NTA's flag is a single
global boolean and stays correct only because NTA effects are baked into stored exams at
generate time; for invigilation we must confirm past invig plans are likewise stored
per-semester (not recomputed live) — otherwise the exclusion needs to be
semester-scoped/time-bounded rather than a single flag.

**Refined direction (Oliver 2026-07-15):** rather than a boolean flag, give each exclusion
a **validity range "gültig von Semester bis Semester"** (validFrom/validUntil, semester
strings; open-ended = still valid). The entry then applies only within its window, so
past semesters stay historically correct regardless of whether invig plans are stored or
recomputed live, and nothing ever has to be deleted or deactivated. Retiring someone = set
validUntil; permanent (pensioniert) = leave validUntil open. Preferred over the NTA-style
single boolean because it sidesteps the stored-vs-live-recompute question entirely.

**BUILT 2026-07-15 (backend, on main working tree — not yet committed):**
- Model `PermanentNonInvigilator` gained `ValidFrom *string` / `ValidUntil *string`
  (semester labels, nil = open) — `graph/model/permanent_non_invigilator.go`.
- `plexams/invigilator_constraints.go`: `semesterOrdinal(label)` (parses "2026 SS" /
  "2026-SS" / "2026SS"; SS sorts before WS of same year → year*2+season),
  `permanentAppliesTo(n, curOrd, curOK)` (errs toward keeping exclusion when a label
  can't be parsed), `normalizeSemesterBound`. `notInvigilating` now filters the permanent
  set by `semesterOrdinal(p.semester)` — so the active workspace semester decides which
  exemptions apply. `SetPermanentNonInvigilator(..., validFrom, validUntil *string)`
  validates each bound + from<=until.
- GraphQL `graph/invigilation.graphqls`: type fields `validFrom`/`validUntil: String`,
  mutation `setPermanentNonInvigilator(..., validFrom: String, validUntil: String)`
  (both optional → backward compatible). Regenerated; resolver in
  `graph/invigilation.resolvers.go`.
- Tests `plexams/invigilator_constraints_test.go` (semesterOrdinal + permanentAppliesTo);
  go build / test / vet / golangci-lint-v2 all clean.
- **GUI-sync pending:** add validFrom/validUntil (semester dropdowns from
  allSemesterNames, both optional) to the permanent-non-invigilator form + show the range
  in the list. Delete is no longer needed for "retired this semester" — set validUntil.

See [[cli-to-gui-migration]].
