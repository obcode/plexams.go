---
name: primuss-xlsx-import
description: Primuss registration data is imported from XLSX via a GUI ZIP upload, replacing the ssconvert+mongoimport Makefile.
metadata:
  type: project
---

Primuss Anmeldedaten (built 2026-06-30, commits 021b2b6, 1990c6b) — replaces the old
Makefile (ssconvert → CSV → mongoimport). Go reads XLSX directly via `excelize`.

- **Upload**: REST `POST /upload/primuss-zip` (multipart field `file`) → `ImportPrimussZip`
  → JSON `{programs:[{program,exams,studentRegs,count,conflicts,missing,changedAncodes}],
  skipped, affectedZpaAncodes}`. Gated by WritesAllowed; marks planning condition
  `primussImported` (phase0). CLI: `primuss import-xlsx <dir>` (`ImportPrimussDir` zips a
  dir then imports). ZIP needs NO special structure: program derived from filename
  (`-<PROG>-[BM]-`), folder layout/.DS_Store/CodeNr-`Prüfungsüberschneidungen` ignored.
- **Incremental**: only programs/collections present in the ZIP are dropped+reinserted.
- **4 collections per program**, byte-identical to mongoimport (verified vs 2026-SS):
  `studentregs_XX` (header already correct; MTKNR string, AnCode int, rest string),
  `exams_XX` (Prüfungskatalog, auto-typed), `count_XX` (Prüfungsplanung, `Sum.`→`Sum`,
  blanks ""), `conflicts_XX` (Prüfungsüberschneidungen_nach_Ancode, blanks omitted).
  Generic db helpers ReplaceRawCollection/RawCollection.
- **Change detection** for update emails: per studentregs replace, compare registrations
  per ancode → changed primuss ancodes; mapped to ZPA ancodes (affectedZpaAncodes) so the
  GUI can send Primuss-data update mails (existing sendEmailPrimussData(ancode, updated)).
- Sammellisten dir is gitignored (real student data — never commit). [[zpa-import-behaviors]]
- **A student can be registered twice for the same exam — do not make that unique.** The
  Primuss source data really contains such duplicates, in the current semester and in
  freshly imported data, so they come straight back with every import. A unique
  constraint would make the import *fail* instead of protecting it; duplicates belong in
  the validation report. PostgreSQL keeps this: `studentreg` has only the lookup indexes
  `studentreg_by_exam_idx`/`studentreg_by_student_idx`
  ([db/migrations/00004_primuss.sql](db/migrations/00004_primuss.sql)). Found 2026-07-30
  under MongoDB (the former db-indexes note).
