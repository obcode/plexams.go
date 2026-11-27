---
name: exam-source-lookup
description: GetZpaExamByAncode also finds external (joint/other-FK) exams -- the exam table holds both; callers must not treat a hit as "is a ZPA exam"
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

**Still latent (not yet seen in practice):** `ConnectPreplanExam`
(preplan_connect.go) and `SetJointZpaLink` (joint_links.go) would accept an
external ancode as a "ZPA exam". The clean fix is `and source = 'zpa'` in
`GetExam`, but check all ~14 callers first (pdf.go, invigilators.go, messages.go,
csv_export.go, rooms_free_seats.go may rely on finding external exams).

Why it surfaced: Michael (planner) could not set the date of a DH exam; the
exam correctly does not appear under „Prüfungen auswählen & Vorgaben“ (that
page lists ZPA exams only). [[mucdai-to-joint-generalization]]
