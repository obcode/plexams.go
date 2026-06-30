---
name: exam-planning-info-email
description: The consolidated per-examer exam-planning info email that replaced the constraints + prepared emails.
metadata:
  type: project
---

The separate "constraints/Wünsche" (sendEmailConstraints) and "vorbereitete Prüfungen"
(sendEmailPrepared) bulk emails were **replaced** by one consolidated, personalized email
(decided + built 2026-06-30, commits 3763600, d856d5a, 74064e4, d5eac53; old ones removed
in d5eac53). Lives in plexams/email_exam_planning.go + tmpl/examPlanningInfoEmail{,HTML}.tmpl.

- **Pre-step query** `examPlanningMailRecipients` (read-only): one entry per examer to let
  the planner select/deselect before sending. Two categories:
  - `withExams`: examer (ANY faculty) with ≥1 exam I plan = toPlan AND not notPlannedByMe;
    includes the exams (ancode/module/examType + constraints, **no slot/date**).
  - `fk07NoExams`: FK07 examer who HAS ≥1 ZPA exam this semester but none I plan (so they
    get a "I plan none of yours" note). Examers without any ZPA exam, and non-FK07 without a
    planned exam, are excluded. (FK07 = Teacher.FK == "FK07".)
- **Send** subscription `sendEmailExamPlanningInfo(run, teacherIDs)` (streams; run=false =
  dry run): per selected examer the matching variant; greeting personalized "Hallo <name>,";
  asks for (further) constraints via Jira + exam period; recipients without email skipped.
  No send-once block (batches/re-send allowed); marks planning condition
  `examPlanningInfoSent` (phase0). CLI: `email exam-planning-info`.
- Source is ZpaExamsToPlanWithConstraints (works pre-Primuss); NOT the plan-entry-based
  ExamersWithExamsPlannedByMe. Recipient selection is the user's safeguard. [[emails-over-graphql]]
