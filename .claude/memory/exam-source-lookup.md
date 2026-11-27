---
name: exam-source-lookup
description: exam table holds ZPA + external exams; GetZpaExamByAncode is ZPA-only (source filter since 2026-11-27), ExamByAncode finds either
metadata:
  type: project
---

Since PostgreSQL, ZPA exams and externally owned exams (joint programs, other
faculties) share the `exam` table, told apart by `source` ('zpa' | 'external').
The `GetExam` query behind `GetZpaExamByAncode` does **not** filter on `source`,
so it returns external exams too. `GetExternalExam` does filter.

**Bug this caused (fixed 2026-11-27, d8fbed9):** `AddExamToSlottime` /
`setExternalExamTime` asked the ZPA lookup first, found a MUC.HEALTH exam planned
by FK11 and refused it with „… plant unsere Fakultät selbst … nicht von mir
geplant“. Now it asks `ExternalExam` first. Regression test:
`TestExternalTimeForJointExam`.

**Clean fix (2026-11-27, same day):** the `GetExam` query now filters
`source = 'zpa'`, so `GetZpaExamByAncode` really is ZPA-only (this closed the
same hole in `ConnectPreplanExam` and `SetJointZpaLink`). Callers that only want
module/examiner of *any* exam use **`Plexams.ExamByAncode`** (ZPA, then external):
invigilators, rooms_free_seats, pdf same-slot, messages, csv_export, PreExamsAt,
ExamInfo, and the `zpaExam` GraphQL resolver -- the GUI's exam page falls back to
it for external ancodes too, so it keeps answering for any source despite its
name. Rule for new code: "is this a ZPA exam?" → `GetZpaExamByAncode`; "what is
this exam?" → `ExamByAncode`.

Why it surfaced: Michael (planner) could not set the date of a DH exam; the
exam correctly does not appear under „Prüfungen auswählen & Vorgaben“ (that
page lists ZPA exams only). [[mucdai-to-joint-generalization]]
