---
name: spread-statistics
description: Student-centric exam-spread quality statistics (gaps between exams) — GraphQL query examSpreadStatistics + aggregate PDF /download/pdf/spread-statistics.
metadata:
  node_type: memory
  type: project
  originSessionId: 0e82f3a9-f051-4a52-b9d6-cf1a5b677f73
---

Feature (built 2026-07-12): "wie gut ist die Terminplanung für die einzelnen Studierenden" — aggregierte Statistik über zeitliche Abstände zwischen aufeinanderfolgenden Prüfungen je Studierende:r.

**Design decisions (from user):** "freier Tag" = KALENDERTAGE (Wochenende zählt als frei). Emailbares PDF = NUR aggregiert (keine Namen); Namens-Drill-down (worstStudents) nur im GUI.

**Population = nur „regular" (eine flache Auswertung):** `examSpreadStatistics` → flaches `ExamSpreadStatistics` über Studierende mit ≤6 NICHT-Wiederholungsprüfungen (maxRegularNonRepeatExams=6, normaler Studienverlauf). Die Ausreißer (>6, Wiederholer) sind NICHT eigene Ansicht, sondern nur Notiz-Felder: `excludedStudentCount` + `allFreeDayShare` (freeDayShare über alle). Grund: der Unterschied regular vs. alle war nicht signifikant (wenige Ausreißer) — Nutzerentscheidung 2026-07, „eine Auswertung + Notiz". [Historie: kurzzeitig gab es regular/all als 2 ExamSpreadScope — wieder verschlankt.] Repeat-Erkennung via `repeatcalc.RepeatForStudent(SemesterOf(student.Group), ex.repeater, MinGroupSemester(ex.Groups))`, examInfoMap liefert repeater+minSem; nonRepeat über ALLE ZpaAncodes gezählt, Gaps über placed. `aggregateScope()` liefert das flache Stat über die regular-Records; all-Kennzahl (allStudents/allMulti/allFree) im Build-Loop akkumuliert. PDF: eine Auswertung + Ausgeblendet-Notiz.

**Welche Anmeldungen zählen (verifiziert):** genau `Student.ZpaAncodes` = laut PrepareStudentRegs (prepare.go:75) NUR Anmeldungen, deren aufgelöster Ancode zu to-plan-ZPA-Exam ODER external Exam gehört. notToPlan-ZPA (Modularbeiten etc., ExamShouldBePlanned nur schriftl./prakt.) und verwaiste MUC.DAI-Primuss-Regs (kein ZPA/external) sind bereits raus. Egal ob von uns oder anderen geplant.

**Population/Scope (wichtig):** nur UNSERE Studierenden = eingeschrieben in FK07- oder MUC.DAI-Programm (`p.zpa.fk07programs` ∪ `p.mucdaiProgramNames(ctx)`), denn nur für die haben wir ALLE Prüfungen im Zeitraum. Timeline = alle zeitlich verplanten Prüfungen (Starttime != nil) im Zeitraum [semesterConfig.From, Until], INKL. external/notPlannedByMe (deren Zeit ist real) — NICHT auf slot-grid beschränkt (onGrid war der ursprüngliche Bug, verwarf genau diese Prüfungen). Fremd-Fakultäts-Studis mit Einzelprüfung bei uns → ausgeschlossen (unvollständige Daten, sonst zu optimistisch).

**Exclusions (like ValidateConflicts):** ein consecutive pair wird komplett verworfen (kein Konflikt), wenn BEIDE Prüfungen foreign sind (>=externalAncodeBase | PlanEntry.External | constraints.NotPlannedByMe) ODER das Paar sameSlot/canShareSlot ist (Falschanmeldung, Studi darf nicht beide). Implementiert als `spreadPairExcluder` → `spreadcalc.ComputeStudent(..., exclude func(a,b))`. Studis ohne wertbares Paar zählen nicht in MultiExamStudentCount/Quoten. maxExamsPerDay zählt distinkte Startzeiten/Tag (kollabiert sameSlot).

**Metrik:** pro Studi placed exams (NTA-korrekte Dauern via examDurationsByAncode) nach Zeit sortiert, consecutive pairs klassifiziert: Überschneidung(-2)/selber Tag(-1)/Folgetag=0 freie Tage/k freie Tage. Headline-Shares = saubere Partition der ≥2-Prüfungs-Studis nach ihrem WORST pair (freeDayShare = min≥1). Carter-Näherungsindex (16/8/4/2/1) als eine Zahl = das was der Solver minimiert.

**Code:**
- `plexams/spreadcalc/` — pure math (ClassifyPair reuses conflictcalc.TimeProximity, PairCost, BucketKey, ComputeStudent) + tests.
- `plexams/spread_statistics.go` — `(*Plexams).ExamSpreadStatistics(ctx)` orchestration (StudentRegsPerStudentPlanned + PlanEntries onGrid filter + examInfoMap).
- `graph/spread_statistics.graphqls` + resolver → query `examSpreadStatistics`.
- `plexams/pdfgen/spread_statistics.go` — anonymous maroto PDF; registered kind `spread-statistics` in pdf_export.go.

Follows the established "same aggregation feeds GraphQL + PDF" pattern (like constraints/preplanOverview). GUI-sync pending: new page rendering ExamSpreadStatistics + PDF download button. See [[terminplan-generator-design]] (solver spread objective this measures), [[validation-conflict-severity]] (same conflictcalc classification).
