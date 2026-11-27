---
name: nta-list-import
description: NTA lists from the exam board's SharePoint folder (PRIMUSS xlsx per faculty) are uploaded, stored per faculty, compared with the NTA master data and applied selectively (2026-11-27).
metadata:
  type: project
---

Since WS 26/27 the exam board no longer mails NTA decisions. It publishes **one xlsx
per faculty** in a SharePoint folder (link on GUI page `/nta/import`, constant
`SHAREPOINT_URL` in `plexams.gui/src/lib/nta/ntaList.js`). Built 2026-11-27.

**The list is incomplete**: it holds only NTAs granted from 2026 on; older ones exist only
in the master data (`nta`). Changes of the exam form are never in it. It grows after every
exam board meeting; the planner re-uploads.

**Format** (package `plexams/ntalist`, pure):
- File name `226_07_Liste_NTA_Stand_27-11-26.xlsx` = semester code, FK, export date.
  FK 21 is MUC.DAI (programs DE-B, ID-B).
- Header block above the column row: export timestamp (I1), `FK 07`, `für Semester 226`.
  Cells win over the file name; a conflict is a warning.
- Columns by name: fk, stg, mtknr, vname, name, beginn_studium, nachteilsausgleich,
  semester, anmeldungen_aktsemester. All cells are strings.
- Semester code `1JJ` = SS 20JJ, `2JJ` = WS 20JJ/JJ+1 (`ntalist.SemesterID`).
- `semester` column: `*` = whole studies, `226:127` = range, the last one counts.
- `nachteilsausgleich` is short free text ("10% Schreibzeitverlängerung, gesonderter
  Raum"). Only `NN % Schreibzeit…` sets the percent; "20% … mündlichen Prüfungen" does
  not. No percent found → row is PROBLEM and the planner sets the fields by hand.
- `anmeldungen_aktsemester`: `IF-B:123 | AW-:456` (program code with degree : Primuss ancode).

**Flow**: `POST /upload/nta-list` (repeated field `file`) stores the parsed list in
`nta_list`/`nta_list_entry`, replacing the previous list of that faculty, keeps
`first_seen` per mtknr, and reports new/changed/removed rows against it. It does NOT touch
`nta`. Query `ntaListPreview` compares each row with `nta` and with `studentreg`; mutation
`applyNTAList` writes the selected rows (all or nothing, via AddNta/UpdateNta).

**Conventions that matter** (found against the real master data):
- `until` follows the stored texts: "Studium IF" for `*`, "Wintersemester 2026/2027" /
  "Sommersemester 2027" for a semester. The store also has "Ende WD", "Studiengang WT",
  "Gesamtes Studium" — all treated as "whole studies" when comparing.
- Differences in wording (text, name, the until variants above) are `significant: false`:
  the row stays UNCHANGED and applying keeps the stored, usually longer, text.
- `from` ("Bescheid vom", DD.MM.YYYY) = export date of the list for new NTAs; kept for existing.
- Email from the ZPA student of the semester if known.
- `p.semester` is the LOGICAL semester ("2026 WS"); compare via `isActiveSemester`.
- Program codes resolve via `primuss.ResolveProgram` (same rules as the Sammellisten names).

Related: [[primuss-xlsx-import]], [[pii-in-fehlerberichten]].
