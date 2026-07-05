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

The user's **accepted conflict decisions flow in**: a pair a student accepted (e.g. registration wrong, student writes only one) is no longer silently skipped — it's shown but **downgraded to Info** ("... (akzeptiert)"). Sources merged into one normalized `accepted` set (`acceptedKey` sorts the ancode pair): DB `StudentConflictDecisions` where Decision==ACCEPT (GUI acceptStudentConflict, the current mechanism) PLUS the legacy `knownConflicts.studentRegs` in the semester YAML (still honored as fallback). A pair with a mix of accepted + real students is graded by the real (non-accepted) ones, "(+N akzeptiert)" noted. The copy-pasteable knownConflicts YAML snippet now only lists not-yet-accepted pairs. Summary line: "N accepted conflicts; E error(s), W warning(s), I info(s)".

Backend-only, no GraphQL schema change (ValidationFinding.Level already exists; report streamed as RESULT line). Same behavior for GUI subscription and CLI `validate conflicts`. See [[terminplan-generator-design]] (conflict ratings), [[graphql-interface-cleanup]].
