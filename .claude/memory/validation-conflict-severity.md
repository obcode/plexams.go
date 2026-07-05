---
name: validation-conflict-severity
description: "ValidateConflicts grades student-conflict findings by severity and folds in the user's accepted conflict decisions."
metadata:
  node_type: memory
  type: project
  originSessionId: 95adce67-93c9-4615-8771-bcf674f5038e
---

`ValidateConflicts` (plexams/validate.go) — settled 2026-07-05.

Findings are graded by proximity, not all-error as before: **same slot → Error** (student can't sit both), **adjacent slot → Warning** (back-to-back; the Terminplan generator already avoids these so they're rare), **same day → Info** (checked at generation time, usually fine). `ValidationReport.Ok`/ErrorCount now reflect only real hard clashes, so same-day no longer blocks the plan. Findings are sorted by original proximity severity, most-severe first (`conflictSeverityRank`: sameSlot<adjacent<sameDay), stable within a rank.

**Everything the user allows is only Info** (`conflictLevel(problem, real, allowed)` — pure, tested). Three allowance mechanisms:
1. **sameSlot constraint** — pair-level: `ConstraintsMap()[a].SameSlot` (ancodes). These exams MUST run together, so a shared student's same-slot is expected.
2. **canShareSlot / canBeSameSlot** — pair-level: `dbClient.CanShareSlotPairs()`.
3. **explicit per-student acceptance** — DB `StudentConflictDecisions` where Decision==ACCEPT (GUI setStudentConflictDecision) + legacy `knownConflicts.studentRegs` YAML (still honored). Normalized `accepted` set via `acceptedKey`.

Pair-level allowances (1,2) downgrade the WHOLE finding to Info regardless of students ("same slot (sameSlot-Constraint/canShareSlot): N ..."); the user does NOT have to accept each student individually. Per-student acceptance (3) downgrades individuals: a pair fully accepted → Info ("... (akzeptiert)"), a mix is graded by the real (non-accepted) students with "(+N akzeptiert)". The knownConflicts YAML snippet skips pair-allowed and fully-accepted pairs. Summary: "N accepted conflicts; E error(s), W warning(s), I info(s)".

**Foreign exams excluded**: a conflict where BOTH exams are foreign (not-planned-by-me constraint OR PlanEntry.ExternalTime OR ancode >= externalAncodeBase=90000, matching examplan's `foreign`) is dropped entirely — not ours to resolve (e.g. two external exams 90017/90023 must not appear at all). `onlyPlannedByMe` now means "only conflicts among our own exams": when true, also drop any pair with one foreign side (was: only both-not-planned). The `foreignAncodes` set replaced the old `planAncodeEntriesNotPlannedByMe` (which only checked the NotPlannedByMe constraint, missing external ancodes). NOTE: `sortConflictingAncodes` may reorder ca by exam time, so `allowedReason` normalizes the pair key (`normPair`).

Backend-only, no GraphQL schema change (ValidationFinding.Level already exists; report streamed as RESULT line). Same behavior for GUI subscription and CLI `validate conflicts`. See [[terminplan-generator-design]] (conflict ratings), [[graphql-interface-cleanup]].
