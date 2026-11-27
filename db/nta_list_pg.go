package db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/obcode/plexams.go/plexams/ntalist"
)

// StoredNTAList is an uploaded NTA list of one faculty as it was read, with the
// time each of its students first appeared in a list of that faculty.
type StoredNTAList struct {
	List       *ntalist.List
	ImportedAt time.Time
	ImportedBy *string
	// FirstSeen is keyed by mtknr.
	FirstSeen map[string]time.Time
}

// NTALists returns the uploaded NTA lists of the semester, ordered by faculty.
func (db *PG) NTALists(ctx context.Context) ([]*StoredNTAList, error) {
	listRows, err := db.q(ctx).ListNtaLists(ctx, db.semesterID)
	if err != nil {
		return nil, fmt.Errorf("cannot read nta lists: %w", err)
	}
	entryRows, err := db.q(ctx).ListNtaListEntries(ctx, db.semesterID)
	if err != nil {
		return nil, fmt.Errorf("cannot read nta list entries: %w", err)
	}

	lists := make([]*StoredNTAList, 0, len(listRows))
	byFK := make(map[string]*StoredNTAList, len(listRows))
	for _, r := range listRows {
		l := &ntalist.List{
			Filename:     r.Filename,
			FK:           r.Fk,
			SemesterCode: r.SemesterCode,
			SemesterID:   r.ListSemester,
			Rows:         []*ntalist.Row{},
			Warnings:     []string{},
		}
		if r.Stand != nil {
			l.Stand = *r.Stand
		}
		if err := json.Unmarshal(r.Warnings, &l.Warnings); err != nil {
			return nil, fmt.Errorf("nta list %s: cannot decode warnings: %w", r.Fk, err)
		}
		s := &StoredNTAList{List: l, ImportedAt: r.ImportedAt, ImportedBy: r.ImportedBy,
			FirstSeen: map[string]time.Time{}}
		lists = append(lists, s)
		byFK[r.Fk] = s
	}
	for _, e := range entryRows {
		s, ok := byFK[e.Fk]
		if !ok {
			continue
		}
		var row ntalist.Row
		if err := json.Unmarshal(e.Row, &row); err != nil {
			return nil, fmt.Errorf("nta list %s line %d: cannot decode row: %w", e.Fk, e.Line, err)
		}
		s.List.Rows = append(s.List.Rows, &row)
		if e.Mtknr != "" {
			s.FirstSeen[e.Mtknr] = e.FirstSeen
		}
	}
	return lists, nil
}

// ReplaceNTAList stores a list, replacing the previous one of the same faculty.
// firstSeen (keyed by mtknr) carries the first appearance over from earlier
// lists; students missing from it are first seen now.
func (db *PG) ReplaceNTAList(ctx context.Context, list *ntalist.List, firstSeen map[string]time.Time, importedBy *string) error {
	warnings, err := json.Marshal(list.Warnings)
	if err != nil {
		return err
	}
	now := time.Now()
	return db.InTransaction(ctx, func(ctx context.Context) error {
		q := db.q(ctx)
		if _, err := q.DeleteNtaList(ctx, sqlc.DeleteNtaListParams{SemesterID: db.semesterID, Fk: list.FK}); err != nil {
			return fmt.Errorf("cannot delete previous nta list: %w", err)
		}
		var stand *time.Time
		if !list.Stand.IsZero() {
			stand = &list.Stand
		}
		if err := q.InsertNtaList(ctx, sqlc.InsertNtaListParams{
			SemesterID:   db.semesterID,
			Fk:           list.FK,
			Filename:     list.Filename,
			SemesterCode: list.SemesterCode,
			ListSemester: list.SemesterID,
			Stand:        stand,
			Warnings:     warnings,
			ImportedBy:   importedBy,
		}); err != nil {
			return fmt.Errorf("cannot insert nta list: %w", err)
		}
		for _, row := range list.Rows {
			data, err := json.Marshal(row)
			if err != nil {
				return err
			}
			seen, ok := firstSeen[row.Mtknr]
			if !ok {
				seen = now
			}
			if err := q.InsertNtaListEntry(ctx, sqlc.InsertNtaListEntryParams{
				SemesterID: db.semesterID,
				Fk:         list.FK,
				Line:       row.Line,
				Mtknr:      row.Mtknr,
				Row:        data,
				FirstSeen:  seen,
			}); err != nil {
				return fmt.Errorf("cannot insert nta list entry (line %d): %w", row.Line, err)
			}
		}
		return nil
	})
}

// PrimussReg is one Primuss registration: the exam's program and its Primuss ancode.
type PrimussReg struct {
	Program       string
	PrimussAncode int
	// StudentProgram is the student's own program (Primuss column Stg).
	StudentProgram string
}

// StudentRegsForMtknrs returns the Primuss registrations of the given students
// in all programs, keyed by mtknr.
func (db *PG) StudentRegsForMtknrs(ctx context.Context, mtknrs []string) (map[string][]PrimussReg, error) {
	rows, err := db.q(ctx).ListStudentRegsForMtknrs(ctx, sqlc.ListStudentRegsForMtknrsParams{
		SemesterID: db.semesterID,
		Mtknrs:     mtknrs,
	})
	if err != nil {
		return nil, fmt.Errorf("cannot read student regs: %w", err)
	}
	out := make(map[string][]PrimussReg)
	for _, r := range rows {
		out[r.Mtknr] = append(out[r.Mtknr], PrimussReg{Program: r.Program, PrimussAncode: r.PrimussAncode, StudentProgram: r.StudentProgram})
	}
	return out, nil
}

// HasStudentRegs reports whether any Primuss registrations were imported for the semester.
func (db *PG) HasStudentRegs(ctx context.Context) (bool, error) {
	n, err := db.q(ctx).CountStudentRegs(ctx, db.semesterID)
	if err != nil {
		return false, fmt.Errorf("cannot count student regs: %w", err)
	}
	return n > 0, nil
}

// RemoveNTAList deletes the list of a faculty; false if there was none.
func (db *PG) RemoveNTAList(ctx context.Context, fk string) (bool, error) {
	n, err := db.q(ctx).DeleteNtaList(ctx, sqlc.DeleteNtaListParams{SemesterID: db.semesterID, Fk: fk})
	if err != nil {
		return false, fmt.Errorf("cannot delete nta list: %w", err)
	}
	return n > 0, nil
}
