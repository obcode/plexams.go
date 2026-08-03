package primuss

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/obcode/plexams.go/db"
)

// The Primuss column names live here, in the importer, and nowhere else.
//
// They used to be storage identifiers: the XLSX header went into MongoDB
// verbatim, which is how `Prüfer` with an umlaut, `Sum.` with a dot and `AnCo`
// without its "de" became field names in the database. A foreign spreadsheet's
// header is an input format, and an input format belongs to the code that reads
// it.
//
// Verified against the 2026-SS Sammellisten -- the four headers really are:
//
//	Prüfungsanmeldungen:  MTKNR AnCode Note Leitsp Stg AASPF Stgru nopos gebucht
//	                      name wie nicht_zul kzwfn datum zeit pon ponbuch praesenz_fern
//	Prüfungskatalog:      AnCode CodeNr Titel Stg kzstr kzsp pruefer kzwpf notart
//	                      sonst ist_praesenz ist_digfern
//	Prüfungsplanung:      AnCo Codenr Titel Prüfer Sum. <one column per study group>
//	Überschneidungen:     AnCo Titel Prüfer <one column per ancode>
//
// Note that the exam catalogue spells the examer `pruefer` and the other two
// files `Prüfer`, and that only the catalogue uses `AnCode` rather than `AnCo`.
const (
	colMtknr    = "MTKNR"
	colAncode   = "AnCode"
	colAncodeSh = "AnCo"
	colStg      = "Stg"
	colStgru    = "Stgru"
	colName     = "name"
	colPresence = "praesenz_fern"
	colAaspf    = "AASPF"

	colTitel       = "Titel"
	colPruefer     = "pruefer"
	colPrueferUml  = "Prüfer"
	colSonst       = "sonst"
	colIstPraesenz = "ist_praesenz"

	colSum = "Sum."
)

// ConvertError is one row the converters could not use. They are collected
// rather than thrown, so one unparseable cell does not lose a whole program's
// import -- but they are no longer silent, which is the point: an ancode that
// failed to parse used to become 0 with nothing but a log.Debug to show for it.
type ConvertError struct {
	Kind   string // studentregs | exams | count | conflicts
	Row    int    // 1-based row in the sheet, header included
	Column string
	Reason string
}

func (e ConvertError) String() string {
	if e.Column != "" {
		return fmt.Sprintf("%s, Zeile %d, Spalte %q: %s", e.Kind, e.Row, e.Column, e.Reason)
	}
	return fmt.Sprintf("%s, Zeile %d: %s", e.Kind, e.Row, e.Reason)
}

// sheet is a parsed XLSX: a trimmed header plus its rows, every row padded to
// the header's width.
type sheet struct {
	header []string
	rows   [][]string
}

func (s *sheet) index(name string) int {
	for i, h := range s.header {
		if h == name {
			return i
		}
	}
	return -1
}

// cell returns the trimmed value of a column, or "" when the column is absent.
func (s *sheet) cell(row []string, name string) string {
	i := s.index(name)
	if i < 0 || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

// raw collects the columns that are not modelled, so nothing the spreadsheet
// carries is lost on the way into a fixed set of columns. Values are typed the
// way mongoimport typed them -- integers become ints -- so a re-export matches
// what the collections held.
func (s *sheet) raw(row []string, modelled map[string]bool) map[string]any {
	out := make(map[string]any, len(s.header))
	for i, name := range s.header {
		if name == "" || modelled[name] || i >= len(row) {
			continue
		}
		val := strings.TrimSpace(row[i])
		if n, err := strconv.Atoi(val); err == nil && val != "" {
			out[name] = n
			continue
		}
		out[name] = val
	}
	return out
}

// convertStudentRegs maps the Prüfungsanmeldungen.
//
// AASPF is parsed to an int here although Primuss delivers it as a string
// ("84"/"90" -- 1984 and 9 occurrences respectively in the 2026-SS IF file). It
// is the key that tells a DC-B registration from a DC-M one, and it belongs in
// the data rather than in a Go switch: see the aaspf_degree table.
func convertStudentRegs(sh *sheet) ([]db.PrimussStudentRegRow, []ConvertError) {
	modelled := map[string]bool{
		colMtknr: true, colAncode: true, colStg: true, colStgru: true,
		colName: true, colPresence: true, colAaspf: true,
	}

	rows := make([]db.PrimussStudentRegRow, 0, len(sh.rows))
	errs := make([]ConvertError, 0)

	for i, row := range sh.rows {
		lineNo := i + 2 // 1-based, header is line 1

		ancode, err := strconv.Atoi(sh.cell(row, colAncode))
		if err != nil {
			errs = append(errs, ConvertError{
				Kind: "studentregs", Row: lineNo, Column: colAncode,
				Reason: fmt.Sprintf("%q ist keine Zahl", sh.cell(row, colAncode)),
			})
			continue
		}
		mtknr := sh.cell(row, colMtknr)
		if mtknr == "" {
			errs = append(errs, ConvertError{
				Kind: "studentregs", Row: lineNo, Column: colMtknr,
				Reason: "leer",
			})
			continue
		}

		reg := db.PrimussStudentRegRow{
			Mtknr:          mtknr,
			PrimussAncode:  ancode,
			StudentProgram: sh.cell(row, colStg),
			Group:          sh.cell(row, colStgru),
			Name:           sh.cell(row, colName),
			Presence:       sh.cell(row, colPresence),
			Raw:            sh.raw(row, modelled),
		}
		if aaspf := sh.cell(row, colAaspf); aaspf != "" {
			n, err := strconv.Atoi(aaspf)
			if err != nil {
				errs = append(errs, ConvertError{
					Kind: "studentregs", Row: lineNo, Column: colAaspf,
					Reason: fmt.Sprintf("%q ist keine Zahl", aaspf),
				})
			} else {
				reg.Aaspf = &n
			}
		}
		rows = append(rows, reg)
	}
	return rows, errs
}

// convertExams maps the Prüfungskatalog.
//
// The `Stg` column stays in raw: it is the exam's own program code without the
// degree suffix ("IF" in a Prüfungskatalog-IF-B file), while the program that
// keys the row is the one from the file name.
func convertExams(sh *sheet) ([]db.PrimussExamRow, []ConvertError) {
	modelled := map[string]bool{
		colAncode: true, colTitel: true, colPruefer: true,
		colSonst: true, colIstPraesenz: true,
	}

	rows := make([]db.PrimussExamRow, 0, len(sh.rows))
	errs := make([]ConvertError, 0)

	for i, row := range sh.rows {
		lineNo := i + 2

		ancode, err := strconv.Atoi(sh.cell(row, colAncode))
		if err != nil {
			errs = append(errs, ConvertError{
				Kind: "exams", Row: lineNo, Column: colAncode,
				Reason: fmt.Sprintf("%q ist keine Zahl", sh.cell(row, colAncode)),
			})
			continue
		}

		rows = append(rows, db.PrimussExamRow{
			Ancode:     ancode,
			Module:     sh.cell(row, colTitel),
			MainExamer: sh.cell(row, colPruefer),
			ExamType:   sh.cell(row, colSonst),
			Presence:   sh.cell(row, colIstPraesenz),
			Raw:        sh.raw(row, modelled),
		})
	}
	return rows, errs
}

// convertCounts maps the Prüfungsplanung: the expected number of students per
// exam, with the per-study-group breakdown kept in raw.
//
// The ancode column is `AnCo` here and the total is `Sum.` with a trailing dot.
// Both were stored under those names; the dot is why the Mongo path carried a
// "sumFix" flag.
func convertCounts(sh *sheet) ([]db.PrimussCountRow, []ConvertError) {
	modelled := map[string]bool{colAncodeSh: true, colSum: true}

	rows := make([]db.PrimussCountRow, 0, len(sh.rows))
	errs := make([]ConvertError, 0)

	for i, row := range sh.rows {
		lineNo := i + 2

		ancode, err := strconv.Atoi(sh.cell(row, colAncodeSh))
		if err != nil {
			errs = append(errs, ConvertError{
				Kind: "count", Row: lineNo, Column: colAncodeSh,
				Reason: fmt.Sprintf("%q ist keine Zahl", sh.cell(row, colAncodeSh)),
			})
			continue
		}

		total := 0
		if sum := sh.cell(row, colSum); sum != "" {
			total, err = strconv.Atoi(sum)
			if err != nil {
				errs = append(errs, ConvertError{
					Kind: "count", Row: lineNo, Column: colSum,
					Reason: fmt.Sprintf("%q ist keine Zahl", sum),
				})
				continue
			}
		}

		rows = append(rows, db.PrimussCountRow{
			Ancode: ancode,
			Total:  total,
			Raw:    sh.raw(row, modelled),
		})
	}
	return rows, errs
}

// convertConflicts pivots the Prüfungsüberschneidungen from wide to long.
//
// The sheet has one column PER ANCODE: row 108 column "112" holds the number of
// students registered for both. That shape is why the ancodes ended up as
// MongoDB field names, and with it the hand-written decoder and the $rename over
// field names. Here each non-empty cell becomes one row.
//
// Two things this no longer does silently:
//
//   - A column header that is not a number used to be skipped with a
//     log.Debug (db/primuss_conflicts.go:146) and the value landed under ancode
//     0. It is now reported.
//   - The same for an unparseable cell value.
//
// The diagonal is kept on purpose: row 108, column "108" is that exam's own
// registration count, and the readers expect it -- 37 such entries in the live
// conflicts of 2026-SS.
func convertConflicts(sh *sheet) ([]db.PrimussConflictRow, []ConvertError) {
	rows := make([]db.PrimussConflictRow, 0)
	errs := make([]ConvertError, 0)

	// Which columns are ancodes, resolved once against the header.
	type ancodeCol struct {
		index  int
		ancode int
	}
	cols := make([]ancodeCol, 0, len(sh.header))
	ancodeIdx := sh.index(colAncodeSh)
	for i, name := range sh.header {
		if i == ancodeIdx || name == "" {
			continue
		}
		other, err := strconv.Atoi(name)
		if err != nil {
			// Titel and Prüfer are expected; anything else is a header we do
			// not understand and must not drop quietly.
			if name == colTitel || name == colPrueferUml || name == colPruefer {
				continue
			}
			errs = append(errs, ConvertError{
				Kind: "conflicts", Row: 1, Column: name,
				Reason: "Spaltenüberschrift ist weder ein Ancode noch Titel/Prüfer",
			})
			continue
		}
		cols = append(cols, ancodeCol{index: i, ancode: other})
	}

	for i, row := range sh.rows {
		lineNo := i + 2

		ancode, err := strconv.Atoi(sh.cell(row, colAncodeSh))
		if err != nil {
			errs = append(errs, ConvertError{
				Kind: "conflicts", Row: lineNo, Column: colAncodeSh,
				Reason: fmt.Sprintf("%q ist keine Zahl", sh.cell(row, colAncodeSh)),
			})
			continue
		}

		for _, col := range cols {
			if col.index >= len(row) {
				continue
			}
			val := strings.TrimSpace(row[col.index])
			if val == "" {
				continue
			}
			num, err := strconv.Atoi(val)
			if err != nil {
				errs = append(errs, ConvertError{
					Kind: "conflicts", Row: lineNo, Column: strconv.Itoa(col.ancode),
					Reason: fmt.Sprintf("%q ist keine Zahl", val),
				})
				continue
			}
			if num == 0 {
				continue // no shared students is not a conflict
			}
			rows = append(rows, db.PrimussConflictRow{
				Ancode:      ancode,
				OtherAncode: col.ancode,
				NumStudents: num,
			})
		}
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Ancode != rows[j].Ancode {
			return rows[i].Ancode < rows[j].Ancode
		}
		return rows[i].OtherAncode < rows[j].OtherAncode
	})
	return rows, errs
}

// dropConflictsWithUnknownExams removes the pairs naming an exam the catalogue
// of this program does not have, and reports them.
//
// The foreign keys would reject them anyway; catching them here turns "the whole
// program's import failed" into a line in the import report, which is what the
// planner can act on. Under MongoDB such a cell was written as a field name that
// no lookup ever resolved.
func dropConflictsWithUnknownExams(rows []db.PrimussConflictRow, known map[int]bool) ([]db.PrimussConflictRow, []ConvertError) {
	kept := make([]db.PrimussConflictRow, 0, len(rows))
	errs := make([]ConvertError, 0)
	reported := make(map[int]bool)

	for _, row := range rows {
		missing := 0
		switch {
		case !known[row.Ancode]:
			missing = row.Ancode
		case !known[row.OtherAncode]:
			missing = row.OtherAncode
		default:
			kept = append(kept, row)
			continue
		}
		if !reported[missing] {
			reported[missing] = true
			errs = append(errs, ConvertError{
				Kind: "conflicts", Column: strconv.Itoa(missing),
				Reason: fmt.Sprintf("Ancode %d steht nicht im Prüfungskatalog des Studiengangs", missing),
			})
		}
	}
	return kept, errs
}
