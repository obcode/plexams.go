package db

import (
	"context"

	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// This file is what Problem 3 of the migration looked like once the ancodes
// stopped being field names.
//
// A conflicts_<PROG> document was `{AnCo, Titel, Prüfer, "<ancode>": n, ...}`. It
// needed a hand-written bson decoder that walked the elements, special-cased the
// three known keys and parsed the rest as integers -- filing anything it could
// not parse under ancode 0 after a log.Debug() nobody reads. All of that is a
// join now, and the decoder is gone with it.

// conflictRow is the shape both queries return: one row per conflict entry, or a
// single row with nulls when the exam has no conflicts at all.
type conflictRow struct {
	ancode      int
	module      string
	mainExamer  string
	otherAncode *int
	numStudents *int
}

func conflictRowsFromForAncode(rows []sqlc.ListPrimussConflictsForAncodeRow) []conflictRow {
	out := make([]conflictRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, conflictRow{
			ancode: row.Ancode, module: row.Module, mainExamer: row.MainExamer,
			otherAncode: row.OtherAncode, numStudents: row.NumStudents,
		})
	}
	return out
}

func conflictRowsFromForProgram(rows []sqlc.ListPrimussConflictsForProgramRow) []conflictRow {
	out := make([]conflictRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, conflictRow{
			ancode: row.Ancode, module: row.Module, mainExamer: row.MainExamer,
			otherAncode: row.OtherAncode, numStudents: row.NumStudents,
		})
	}
	return out
}

// conflictsFromRows groups the rows of one exam. The query already orders by the
// counterpart ancode, which is the sort conflictToModelConflicts used to do in Go.
func conflictsFromRows(rows []conflictRow) *model.Conflicts {
	if len(rows) == 0 {
		return nil
	}

	conflicts := make([]*model.Conflict, 0, len(rows))
	for _, row := range rows {
		// The left join produced a placeholder row: the exam exists, it just has
		// no conflicts.
		if row.otherAncode == nil || row.numStudents == nil {
			continue
		}
		conflicts = append(conflicts, &model.Conflict{
			AnCode:        *row.otherAncode,
			NumberOfStuds: *row.numStudents,
		})
	}

	return &model.Conflicts{
		AnCode:     rows[0].ancode,
		Module:     rows[0].module,
		MainExamer: rows[0].mainExamer,
		Conflicts:  conflicts,
	}
}

// GetPrimussConflictsForAncodeOnlyPlanned narrows the conflicts to the exams that
// are actually being planned. Unchanged from the Mongo version -- it filters in
// memory over a list the caller already holds.
func (db *PG) GetPrimussConflictsForAncodeOnlyPlanned(ctx context.Context, program string, ancode int,
	zpaExamsToPlan []*model.ZPAExam,
) (*model.Conflicts, error) {
	conflicts, err := db.GetPrimussConflictsForAncode(ctx, program, ancode)
	if err != nil {
		log.Error().Err(err).Str("program", program).Int("ancode", ancode).Msg("cannot get conflicts")
		return nil, err
	}

	conflictsNeeded := make([]*model.Conflict, 0)
	for _, conflict := range conflicts.Conflicts {
		for _, exam := range zpaExamsToPlan {
			if conflict.AnCode == exam.AnCode {
				conflictsNeeded = append(conflictsNeeded, conflict)
				break
			}
		}
	}
	conflicts.Conflicts = conflictsNeeded

	return conflicts, nil
}

// GetPrimussConflictsForAncode returns the conflicts of one Primuss exam.
//
// An exam without conflicts yields an empty list, not nil and not an error --
// under MongoDB it had a conflicts document with no counterpart keys, and callers
// read the module and examer off the result either way.
func (db *PG) GetPrimussConflictsForAncode(ctx context.Context, program string, ancode int) (*model.Conflicts, error) {
	rows, err := db.q(ctx).ListPrimussConflictsForAncode(ctx, sqlc.ListPrimussConflictsForAncodeParams{
		SemesterID: db.semesterID,
		Program:    program,
		Ancode:     ancode,
	})
	if err != nil {
		log.Error().Err(err).Str("program", program).Int("ancode", ancode).
			Msg("cannot get conflicts for ancode")
		return nil, err
	}

	conflicts := conflictsFromRows(conflictRowsFromForAncode(rows))
	if conflicts == nil {
		// No exam, hence nothing to report. The Mongo version surfaced
		// ErrNoDocuments here; an empty result is the same statement without
		// making the caller know about the driver.
		return &model.Conflicts{AnCode: ancode, Conflicts: make([]*model.Conflict, 0)}, nil
	}
	return conflicts, nil
}

// GetPrimussConflictsPerAncode returns all conflicts of a program at once, keyed
// by ancode. Used to compute assembled exams without a per-exam DB lookup.
func (db *PG) GetPrimussConflictsPerAncode(ctx context.Context, program string) (map[int]*model.Conflicts, error) {
	rows, err := db.q(ctx).ListPrimussConflictsForProgram(ctx, sqlc.ListPrimussConflictsForProgramParams{
		SemesterID: db.semesterID,
		Program:    program,
	})
	if err != nil {
		log.Error().Err(err).Str("program", program).Msg("cannot get conflicts")
		return nil, err
	}

	result := make(map[int]*model.Conflicts)
	all := conflictRowsFromForProgram(rows)
	for start := 0; start < len(all); {
		end := start
		for end < len(all) && all[end].ancode == all[start].ancode {
			end++
		}
		conflicts := conflictsFromRows(all[start:end])
		result[conflicts.AnCode] = conflicts
		start = end
	}

	return result, nil
}

// ChangeAncodeInConflicts reports the conflicts of a renumbered exam.
//
// The rename itself no longer happens here: ChangeAncode updates primuss_exam and
// both conflict keys follow by ON UPDATE CASCADE, including the entries that name
// the exam as a counterpart. That cascade is what replaced the $rename over field
// names, so by the time FixPrimussAncode reaches this method the work is done and
// all that is left is to read the result back.
//
// Returns (nil, nil) when there is no exam under the new ancode, which is the
// outcome the Mongo version produced when its filter matched nothing.
func (db *PG) ChangeAncodeInConflicts(ctx context.Context, program string, ancode, newAncode int) (*model.Conflicts, error) {
	rows, err := db.q(ctx).ListPrimussConflictsForAncode(ctx, sqlc.ListPrimussConflictsForAncodeParams{
		SemesterID: db.semesterID,
		Program:    program,
		Ancode:     newAncode,
	})
	if err != nil {
		log.Error().Err(err).Str("program", program).Int("from", ancode).Int("to", newAncode).
			Msg("cannot read back the renumbered conflicts")
		return nil, err
	}

	conflicts := conflictsFromRows(conflictRowsFromForAncode(rows))
	if conflicts == nil {
		log.Debug().Str("program", program).Int("from", ancode).Int("to", newAncode).
			Msg("no conflicts updated while trying to change ancode")
		return nil, nil
	}
	return conflicts, nil
}
