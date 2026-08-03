package db

import (
	"context"
	"fmt"
)

// The typed Primuss import payloads. These replace the []map[string]any that
// travelled from the XLSX parser straight into MongoDB: the column names of a
// foreign spreadsheet used to be storage identifiers, which is why `Prüfer` with
// an umlaut and `Sum.` with a dot were field names in the database.
//
// They live here rather than in plexams/primuss because the importer imports
// this package, not the other way round -- the same reason db.EmailAttachment
// and db.JointLink live here.
//
// Every row keeps a Raw map of the XLSX columns that are not modelled. Under
// MongoDB unknown columns were stored implicitly; dropping them on the way to a
// fixed set of columns would be a silent regression, and the `raw jsonb` columns
// exist for exactly this.

// PrimussExamRow is one row of the Prüfungskatalog.
type PrimussExamRow struct {
	Ancode     int
	Module     string
	MainExamer string
	ExamType   string
	Presence   string
	Raw        map[string]any
}

// PrimussStudentRegRow is one row of the Prüfungsanmeldungen.
type PrimussStudentRegRow struct {
	Mtknr         string
	PrimussAncode int
	// StudentProgram is the registration's own `Stg`: the program of the
	// STUDENT, which is not the program of the exam. In 2026-SS they differ for
	// 178 of 10794 registrations -- see the note on studentreg.student_program.
	StudentProgram string
	Group          string
	Name           string
	Presence       string
	// Aaspf is the Primuss degree key (84 = Bachelor, 90 = Master). Nil when the
	// column is absent or unparseable rather than 0, which would look like a
	// real code.
	Aaspf *int
	Raw   map[string]any
}

// PrimussCountRow is one row of the Prüfungsplanung: the expected number of
// students per exam, plus the per-study-group breakdown in Raw.
type PrimussCountRow struct {
	Ancode int
	Total  int
	Raw    map[string]any
}

// PrimussConflictRow is one cell of the pivoted Prüfungsüberschneidungen: how
// many students are registered for both exams. The diagonal (Ancode ==
// OtherAncode) is kept -- there it is the exam's own registration count, which
// is what the readers expect.
type PrimussConflictRow struct {
	Ancode      int
	OtherAncode int
	NumStudents int
}

// primussExamDoc turns a typed row back into the document shape the MongoDB
// collections use, so the Mongo layer keeps writing exactly what it wrote
// before. These four adapters exist only until the flip; they die with the Mongo
// layer, together with ReplaceRawCollection underneath them.
func primussExamDoc(program string, row PrimussExamRow) map[string]any {
	doc := cloneRaw(row.Raw)
	doc["AnCode"] = row.Ancode
	doc["Titel"] = row.Module
	doc["pruefer"] = row.MainExamer
	doc["sonst"] = row.ExamType
	doc["ist_praesenz"] = row.Presence
	if _, ok := doc["Stg"]; !ok {
		doc["Stg"] = program
	}
	return doc
}

func primussStudentRegDoc(row PrimussStudentRegRow) map[string]any {
	doc := cloneRaw(row.Raw)
	doc["MTKNR"] = row.Mtknr
	doc["AnCode"] = row.PrimussAncode
	doc["Stg"] = row.StudentProgram
	doc["Stgru"] = row.Group
	doc["name"] = row.Name
	doc["praesenz_fern"] = row.Presence
	return doc
}

func primussCountDoc(row PrimussCountRow) map[string]any {
	doc := cloneRaw(row.Raw)
	doc["AnCo"] = row.Ancode
	doc["Sum"] = row.Total
	return doc
}

func cloneRaw(raw map[string]any) map[string]any {
	doc := make(map[string]any, len(raw)+6)
	for k, v := range raw {
		doc[k] = v
	}
	return doc
}

// ReplacePrimussExams writes the Prüfungskatalog of one program.
func (db *DB) ReplacePrimussExams(ctx context.Context, program string, rows []PrimussExamRow) (int, error) {
	docs := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		docs = append(docs, primussExamDoc(program, row))
	}
	return db.ReplaceRawCollection(ctx, "exams_"+program, docs)
}

// ReplacePrimussStudentRegs writes the Prüfungsanmeldungen of one program.
func (db *DB) ReplacePrimussStudentRegs(ctx context.Context, program string, rows []PrimussStudentRegRow) (int, error) {
	docs := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		docs = append(docs, primussStudentRegDoc(row))
	}
	return db.ReplaceRawCollection(ctx, "studentregs_"+program, docs)
}

// ReplacePrimussCounts writes the Prüfungsplanung of one program.
func (db *DB) ReplacePrimussCounts(ctx context.Context, program string, rows []PrimussCountRow) (int, error) {
	docs := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		docs = append(docs, primussCountDoc(row))
	}
	return db.ReplaceRawCollection(ctx, "count_"+program, docs)
}

// ReplacePrimussConflicts writes the conflicts of one program.
//
// The Mongo layer stores them the way it always did: one document per exam with
// the counterpart ancodes as FIELD NAMES. That is the shape whose decoder
// db/primuss_conflicts.go:122 exists for, and it is the shape that dies at the
// flip -- the pivoting happens in the importer now, so this adapter has to fold
// the long rows back into wide documents.
func (db *DB) ReplacePrimussConflicts(ctx context.Context, program string, rows []PrimussConflictRow) (int, error) {
	byAncode := make(map[int]map[string]any)
	order := make([]int, 0)
	for _, row := range rows {
		doc, ok := byAncode[row.Ancode]
		if !ok {
			doc = map[string]any{"AnCo": row.Ancode}
			byAncode[row.Ancode] = doc
			order = append(order, row.Ancode)
		}
		doc[fmt.Sprintf("%d", row.OtherAncode)] = row.NumStudents
	}

	docs := make([]map[string]any, 0, len(order))
	for _, ancode := range order {
		docs = append(docs, byAncode[ancode])
	}
	return db.ReplaceRawCollection(ctx, "conflicts_"+program, docs)
}

// PrimussStudentRegRows reads back the stored registrations of a program, for
// the change detection on re-import.
func (db *DB) PrimussStudentRegRows(ctx context.Context, program string) ([]PrimussStudentRegRow, error) {
	docs, err := db.RawCollection(ctx, "studentregs_"+program)
	if err != nil {
		return nil, err
	}

	rows := make([]PrimussStudentRegRow, 0, len(docs))
	for _, doc := range docs {
		rows = append(rows, PrimussStudentRegRow{
			Mtknr:          docString(doc, "MTKNR"),
			PrimussAncode:  docInt(doc, "AnCode"),
			StudentProgram: docString(doc, "Stg"),
			Group:          docString(doc, "Stgru"),
			Name:           docString(doc, "name"),
			Presence:       docString(doc, "praesenz_fern"),
			Raw:            doc,
		})
	}
	return rows, nil
}

func docString(doc map[string]any, key string) string {
	if s, ok := doc[key].(string); ok {
		return s
	}
	return ""
}

func docInt(doc map[string]any, key string) int {
	switch n := doc[key].(type) {
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}
