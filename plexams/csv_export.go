package plexams

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// This file implements a human-readable CSV export/import of the data the planner
// enters by hand (constraints, external/MUC.DAI exam times, pre-plan, room requests,
// exam-to-plan selection, duration overrides, conflict ratings). Unlike the JSON
// semester dump it stores ABSOLUTE date/time (not period-relative slot numbers), so a
// re-import stays correct after the exam period shifts: times are fed back through
// SetExternalExamTime, which stores the absolute Starttime directly (time-based model).
//
// Safety: imports never drop a whole collection. Row-keyed datasets upsert per row
// (missing rows are simply not touched); the only full-replace dataset (room requests)
// refuses an empty file. Every import first checks the CSV header matches the dataset,
// so a file cannot be applied to the wrong dataset.

const (
	csvDateLayout     = "02.01.2006"
	csvDateTimeLayout = "02.01.2006 15:04"
	csvListSep        = ";" // separator for multi-value cells (the CSV delimiter is ",")
)

// CSVImportResult reports the outcome of a dataset CSV import.
type CSVImportResult struct {
	Dataset string   `json:"dataset"`
	Applied int      `json:"applied"`
	Skipped []string `json:"skipped,omitempty"`
}

func newCSVImportResult(name string) *CSVImportResult {
	return &CSVImportResult{Dataset: name, Skipped: make([]string, 0)}
}

// csvDataset defines one human-readable dataset: its columns, how to build the rows,
// and how to apply an uploaded file.
type csvDataset struct {
	Title      string
	File       string
	Header     []string
	exportRows func(ctx context.Context) ([][]string, error)
	importRows func(ctx context.Context, rows [][]string) (*CSVImportResult, error)
}

// csvDatasetOrder is the stable order used for the combined "my inputs" ZIP and for
// listing.
var csvDatasetOrder = []string{
	"constraints",
	"external-exams",
	"exam-times",
	"preplan",
	"room-requests",
	"exams-to-plan",
	"duration-overrides",
	"conflict-ratings",
	"can-share-slot",
}

// CSVDatasetNames returns the known CSV dataset keys in a stable order.
func CSVDatasetNames() []string {
	out := make([]string, len(csvDatasetOrder))
	copy(out, csvDatasetOrder)
	return out
}

// ---- small formatting/parsing helpers ----------------------------------------

func b2s(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func s2b(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "x", "ja", "yes", "y", "wahr":
		return true
	}
	return false
}

func strPtr2s(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func s2strPtr(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func intPtr2s(p *int) string {
	if p == nil {
		return ""
	}
	return strconv.Itoa(*p)
}

func s2intPtr(s string) (*int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func ints2s(xs []int) string {
	if len(xs) == 0 {
		return ""
	}
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = strconv.Itoa(x)
	}
	return strings.Join(parts, csvListSep)
}

func s2ints(s string) ([]int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, csvListSep)
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

func strs2s(xs []string) string {
	return strings.Join(xs, csvListSep)
}

func s2strs(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, csvListSep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func datePtr2s(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(csvDateLayout)
}

func dateTimePtr2s(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(csvDateTimeLayout)
}

func dates2s(ts []*time.Time) string {
	if len(ts) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ts))
	for _, t := range ts {
		if t != nil {
			parts = append(parts, t.Format(csvDateLayout))
		}
	}
	return strings.Join(parts, csvListSep)
}

func s2datePtr(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	t, err := time.ParseInLocation(csvDateLayout, s, time.Local)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func s2dateTimePtr(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	t, err := time.ParseInLocation(csvDateTimeLayout, s, time.Local)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func s2dates(s string) ([]*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, csvListSep)
	out := make([]*time.Time, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		t, err := time.ParseInLocation(csvDateLayout, p, time.Local)
		if err != nil {
			return nil, err
		}
		tt := t
		out = append(out, &tt)
	}
	return out, nil
}

// cell safely returns column i of a row (empty if out of range).
func cell(row []string, i int) string {
	if i < len(row) {
		return strings.TrimSpace(row[i])
	}
	return ""
}

// ---- CSV encode/decode --------------------------------------------------------

func encodeCSV(header []string, rows [][]string) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("\xEF\xBB\xBF") // UTF-8 BOM so Excel shows umlauts correctly
	w := csv.NewWriter(&buf)
	if err := w.Write(header); err != nil {
		return nil, err
	}
	if err := w.WriteAll(rows); err != nil {
		return nil, err
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// decodeCSV parses an uploaded CSV, strips a UTF-8 BOM, verifies the header matches the
// dataset (guarding against a file uploaded to the wrong dataset) and returns the data
// rows (without the header).
func decodeCSV(data []byte, expectedHeader []string) ([][]string, error) {
	data = bytes.TrimPrefix(data, []byte("\xEF\xBB\xBF"))
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1 // tolerate short rows; we index defensively
	r.TrimLeadingSpace = true
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("cannot parse CSV: %w", err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("empty CSV (no header)")
	}
	if !headerMatches(records[0], expectedHeader) {
		return nil, fmt.Errorf("unexpected columns %v, expected %v — is this the right dataset?",
			records[0], expectedHeader)
	}
	return records[1:], nil
}

func headerMatches(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if !strings.EqualFold(strings.TrimSpace(got[i]), want[i]) {
			return false
		}
	}
	return true
}

// ---- dataset registry ---------------------------------------------------------

func (p *Plexams) csvDatasets() map[string]csvDataset {
	return map[string]csvDataset{
		"constraints":        p.csvConstraints(),
		"external-exams":     p.csvExternalExams(),
		"exam-times":         p.csvExamTimes(),
		"preplan":            p.csvPreplan(),
		"room-requests":      p.csvRoomRequests(),
		"exams-to-plan":      p.csvExamsToPlan(),
		"duration-overrides": p.csvDurationOverrides(),
		"conflict-ratings":   p.csvConflictRatings(),
		"can-share-slot":     p.csvCanShareSlot(),
	}
}

// ---- constraints --------------------------------------------------------------

func (p *Plexams) csvConstraints() csvDataset {
	header := []string{
		"ancode", "notPlannedByMe", "notPlannedByMeInFK", "online", "location", "doNotPublish",
		"excludeDays", "possibleDays", "fixedDay", "fixedTime", "sameSlot",
		"allowedRooms", "placesWithSocket", "lab", "exahm", "seb", "kdpJiraURL",
		"maxStudents", "additionalSeats", "comments",
	}
	return csvDataset{
		Title:  "Constraints (inkl. notPlannedByMe)",
		File:   "constraints.csv",
		Header: header,
		exportRows: func(ctx context.Context) ([][]string, error) {
			cs, err := p.dbClient.GetConstraints(ctx)
			if err != nil {
				return nil, err
			}
			rows := make([][]string, 0, len(cs))
			for _, c := range cs {
				rc := c.RoomConstraints
				var allowed, socket, lab, exahm, seb, kdp, maxStud, addSeats, comments string
				if rc != nil {
					allowed = strs2s(rc.AllowedRooms)
					socket, lab, exahm, seb = b2s(rc.PlacesWithSocket), b2s(rc.Lab), b2s(rc.Exahm), b2s(rc.Seb)
					kdp = strPtr2s(rc.KdpJiraURL)
					maxStud = intPtr2s(rc.MaxStudents)
					addSeats = intPtr2s(rc.AdditionalSeats)
					comments = strPtr2s(rc.Comments)
				}
				rows = append(rows, []string{
					strconv.Itoa(c.Ancode), b2s(c.NotPlannedByMe), strPtr2s(c.NotPlannedByMeInFk), b2s(c.Online),
					strPtr2s(c.Location), b2s(c.DoNotPublish),
					dates2s(c.ExcludeDays), dates2s(c.PossibleDays), datePtr2s(c.FixedDay), dateTimePtr2s(c.FixedTime),
					ints2s(c.SameSlot),
					allowed, socket, lab, exahm, seb, kdp, maxStud, addSeats, comments,
				})
			}
			return rows, nil
		},
		importRows: func(ctx context.Context, rows [][]string) (*CSVImportResult, error) {
			res := newCSVImportResult("constraints")
			for i, row := range rows {
				ancode, err := strconv.Atoi(cell(row, 0))
				if err != nil {
					res.Skipped = append(res.Skipped, fmt.Sprintf("Zeile %d: ungültiger ancode %q", i+2, cell(row, 0)))
					continue
				}
				excl, err1 := s2dates(cell(row, 6))
				poss, err2 := s2dates(cell(row, 7))
				fixedDay, err3 := s2datePtr(cell(row, 8))
				fixedTime, err4 := s2dateTimePtr(cell(row, 9))
				sameSlot, err5 := s2ints(cell(row, 10))
				maxStud, err6 := s2intPtr(cell(row, 17))
				addSeats, err7 := s2intPtr(cell(row, 18))
				if err := firstErr(err1, err2, err3, err4, err5, err6, err7); err != nil {
					res.Skipped = append(res.Skipped, fmt.Sprintf("Zeile %d (ancode %d): %v", i+2, ancode, err))
					continue
				}

				var rc *model.RoomConstraints
				allowed := s2strs(cell(row, 11))
				socket, lab, exahm, seb := s2b(cell(row, 12)), s2b(cell(row, 13)), s2b(cell(row, 14)), s2b(cell(row, 15))
				kdp := s2strPtr(cell(row, 16))
				comments := s2strPtr(cell(row, 19))
				if len(allowed) > 0 || socket || lab || exahm || seb || kdp != nil || maxStud != nil || addSeats != nil || comments != nil {
					rc = &model.RoomConstraints{
						AllowedRooms: allowed, PlacesWithSocket: socket, Lab: lab, Exahm: exahm, Seb: seb,
						KdpJiraURL: kdp, MaxStudents: maxStud, AdditionalSeats: addSeats, Comments: comments,
					}
				}

				c := &model.Constraints{
					Ancode:             ancode,
					NotPlannedByMe:     s2b(cell(row, 1)),
					NotPlannedByMeInFk: s2strPtr(cell(row, 2)),
					Online:             s2b(cell(row, 3)),
					Location:           s2strPtr(cell(row, 4)),
					DoNotPublish:       s2b(cell(row, 5)),
					ExcludeDays:        excl,
					PossibleDays:       poss,
					FixedDay:           fixedDay,
					FixedTime:          fixedTime,
					SameSlot:           sameSlot,
					RoomConstraints:    rc,
				}
				if _, err := p.dbClient.AddConstraints(ctx, ancode, c); err != nil {
					res.Skipped = append(res.Skipped, fmt.Sprintf("Zeile %d (ancode %d): %v", i+2, ancode, err))
					continue
				}
				res.Applied++
			}
			return res, nil
		},
	}
}

// ---- external exams (shells, without time) ------------------------------------

func (p *Plexams) csvExternalExams() csvDataset {
	header := []string{
		"ancode", "module", "mainExamer", "mainExamerID", "examType", "examTypeFull",
		"duration", "isRepeaterExam", "groups", "faculty", "primussAncodes",
	}
	return csvDataset{
		Title:  "Externe Prüfungen (Stammdaten)",
		File:   "external-exams.csv",
		Header: header,
		exportRows: func(ctx context.Context) ([][]string, error) {
			exams, err := p.dbClient.ExternalExams(ctx)
			if err != nil {
				return nil, err
			}
			rows := make([][]string, 0, len(exams))
			for _, e := range exams {
				rows = append(rows, []string{
					strconv.Itoa(e.AnCode), e.Module, e.MainExamer, strconv.Itoa(e.MainExamerID),
					e.ExamType, e.ExamTypeFull, strconv.Itoa(e.Duration), b2s(e.IsRepeaterExam),
					strs2s(e.Groups), e.Faculty, primussAncodes2s(e.PrimussAncodes),
				})
			}
			return rows, nil
		},
		importRows: func(ctx context.Context, rows [][]string) (*CSVImportResult, error) {
			res := newCSVImportResult("external-exams")
			for i, row := range rows {
				ancode, err := strconv.Atoi(cell(row, 0))
				if err != nil {
					res.Skipped = append(res.Skipped, fmt.Sprintf("Zeile %d: ungültiger ancode %q", i+2, cell(row, 0)))
					continue
				}
				duration, _ := strconv.Atoi(cell(row, 6))
				mainExamerID, _ := strconv.Atoi(cell(row, 3))
				primuss, err := s2primussAncodes(cell(row, 10))
				if err != nil {
					res.Skipped = append(res.Skipped, fmt.Sprintf("Zeile %d (ancode %d): %v", i+2, ancode, err))
					continue
				}
				exam := &model.ZPAExam{
					Semester: p.semester, AnCode: ancode, Module: cell(row, 1), MainExamer: cell(row, 2),
					MainExamerID: mainExamerID, ExamType: cell(row, 4), ExamTypeFull: cell(row, 5),
					Duration: duration, IsRepeaterExam: s2b(cell(row, 7)), Groups: s2strs(cell(row, 8)),
					Faculty: cell(row, 9), PrimussAncodes: primuss,
				}
				// upsert: remove any existing external exam with this ancode, then add.
				_ = p.dbClient.DeleteExternalExam(ctx, ancode)
				if err := p.dbClient.AddExternalExam(ctx, exam); err != nil {
					res.Skipped = append(res.Skipped, fmt.Sprintf("Zeile %d (ancode %d): %v", i+2, ancode, err))
					continue
				}
				res.Applied++
			}
			return res, nil
		},
	}
}

func primussAncodes2s(ps []model.ZPAPrimussAncodes) string {
	if len(ps) == 0 {
		return ""
	}
	parts := make([]string, len(ps))
	for i, pa := range ps {
		parts[i] = fmt.Sprintf("%s:%d", pa.Program, pa.Ancode)
	}
	return strings.Join(parts, csvListSep)
}

func s2primussAncodes(s string) ([]model.ZPAPrimussAncodes, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, csvListSep)
	out := make([]model.ZPAPrimussAncodes, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		kv := strings.SplitN(p, ":", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("ungültiger primussAncode %q (erwartet Programm:ancode)", p)
		}
		ancode, err := strconv.Atoi(strings.TrimSpace(kv[1]))
		if err != nil {
			return nil, fmt.Errorf("ungültiger primussAncode %q: %w", p, err)
		}
		out = append(out, model.ZPAPrimussAncodes{Program: strings.TrimSpace(kv[0]), Ancode: ancode})
	}
	return out, nil
}

// ---- exam times (absolute date/time; external + notPlannedByMe MUC.DAI) --------

func (p *Plexams) csvExamTimes() csvDataset {
	// ancode is the internal ZPA/external ancode (the round-trip key for the import).
	// program + primussAncode are read-only clarity columns: the external (Primuss/MUC.DAI)
	// identity a human/MUC.DAI reader expects (e.g. DE/202); the import ignores them.
	header := []string{"ancode", "module", "program", "primussAncode", "date", "time"}
	return csvDataset{
		Title:  "Prüfungszeiten (extern/MUC.DAI, absolut)",
		File:   "exam-times.csv",
		Header: header,
		exportRows: func(ctx context.Context) ([][]string, error) {
			entries, err := p.dbClient.PlanEntries(ctx)
			if err != nil {
				return nil, err
			}
			rows := make([][]string, 0)
			for _, e := range entries {
				if !e.External || e.Starttime == nil {
					continue
				}
				program, primussAncode := p.externalPrimussIdentity(ctx, e.Ancode)
				primussStr := ""
				if primussAncode > 0 {
					primussStr = strconv.Itoa(primussAncode)
				}
				rows = append(rows, []string{
					strconv.Itoa(e.Ancode), p.moduleForAncode(ctx, e.Ancode),
					program, primussStr,
					e.Starttime.Format(csvDateLayout), e.Starttime.Format("15:04"),
				})
			}
			sort.Slice(rows, func(i, j int) bool { return rows[i][0] < rows[j][0] })
			return rows, nil
		},
		importRows: func(ctx context.Context, rows [][]string) (*CSVImportResult, error) {
			res := newCSVImportResult("exam-times")
			for i, row := range rows {
				ancode, err := strconv.Atoi(cell(row, 0))
				if err != nil {
					res.Skipped = append(res.Skipped, fmt.Sprintf("Zeile %d: ungültiger ancode %q", i+2, cell(row, 0)))
					continue
				}
				// program (col 2) + primussAncode (col 3) are read-only clarity columns and
				// are intentionally ignored on import — the ancode column is the key.
				date, t := cell(row, 4), cell(row, 5)
				if date == "" || t == "" {
					res.Skipped = append(res.Skipped, fmt.Sprintf("Zeile %d (ancode %d): Datum/Uhrzeit fehlt", i+2, ancode))
					continue
				}
				// SetExternalExamTime stores the absolute Starttime directly, so a period
				// shift no longer misplaces the exam.
				if _, err := p.SetExternalExamTime(ctx, ancode, date, t); err != nil {
					res.Skipped = append(res.Skipped, fmt.Sprintf("Zeile %d (ancode %d): %v", i+2, ancode, err))
					continue
				}
				res.Applied++
			}
			return res, nil
		},
	}
}

// moduleForAncode returns a human-friendly module name for an ancode (best effort).
func (p *Plexams) moduleForAncode(ctx context.Context, ancode int) string {
	if ext, err := p.dbClient.ExternalExam(ctx, ancode); err == nil && ext != nil {
		return ext.Module
	}
	if zpa, err := p.GetZpaExamByAncode(ctx, ancode); err == nil && zpa != nil {
		return zpa.Module
	}
	return ""
}

// externalPrimussIdentity returns the external (Primuss/MUC.DAI) identity — program and
// Primuss ancode — of an external exam given its internal ancode. It reads the exam's first
// PrimussAncodes entry; returns ("", 0) if the exam or the entry is missing.
func (p *Plexams) externalPrimussIdentity(ctx context.Context, ancode int) (string, int) {
	ext, err := p.dbClient.ExternalExam(ctx, ancode)
	if err != nil || ext == nil || len(ext.PrimussAncodes) == 0 {
		return "", 0
	}
	pa := ext.PrimussAncodes[0]
	return pa.Program, pa.Ancode
}

// ---- preplan ------------------------------------------------------------------

func (p *Plexams) csvPreplan() csvDataset {
	header := []string{
		"id", "examKind", "examerID", "examerName", "module", "programs", "expectedStudents",
		"duration", "plannedDate", "plannedTime", "isFixed", "notSameSlot", "canShareSlot", "ancode", "notes",
	}
	return csvDataset{
		Title:  "Vorplanung (SEB/EXaHM)",
		File:   "preplan.csv",
		Header: header,
		exportRows: func(ctx context.Context) ([][]string, error) {
			exams, err := p.dbClient.PreplanExams(ctx)
			if err != nil {
				return nil, err
			}
			rows := make([][]string, 0, len(exams))
			for _, e := range exams {
				plannedDate, plannedTime := "", ""
				if e.PlannedStarttime != nil {
					plannedDate, plannedTime = e.PlannedStarttime.Format(csvDateLayout), e.PlannedStarttime.Format("15:04")
				}
				rows = append(rows, []string{
					strconv.Itoa(e.ID), e.ExamKind, strconv.Itoa(e.ExamerID), e.ExamerName, e.Module,
					strs2s(e.Programs), strconv.Itoa(e.ExpectedStudents), intPtr2s(e.Duration),
					plannedDate, plannedTime, b2s(e.IsFixed), ints2s(e.NotSameSlot), ints2s(e.CanShareSlot),
					intPtr2s(e.Ancode), e.Notes,
				})
			}
			return rows, nil
		},
		importRows: func(ctx context.Context, rows [][]string) (*CSVImportResult, error) {
			res := newCSVImportResult("preplan")
			for i, row := range rows {
				id, err := strconv.Atoi(cell(row, 0))
				if err != nil {
					res.Skipped = append(res.Skipped, fmt.Sprintf("Zeile %d: ungültige id %q", i+2, cell(row, 0)))
					continue
				}
				examerID, _ := strconv.Atoi(cell(row, 2))
				expected, _ := strconv.Atoi(cell(row, 6))
				duration, errD := s2intPtr(cell(row, 7))
				notSame, errN := s2ints(cell(row, 11))
				canShare, errC := s2ints(cell(row, 12))
				ancode, errA := s2intPtr(cell(row, 13))
				if err := firstErr(errD, errN, errC, errA); err != nil {
					res.Skipped = append(res.Skipped, fmt.Sprintf("Zeile %d (id %d): %v", i+2, id, err))
					continue
				}

				var plannedStarttime *time.Time
				if date, t := cell(row, 8), cell(row, 9); date != "" && t != "" {
					when, err := time.ParseInLocation(csvDateTimeLayout, date+" "+t, time.Local)
					if err != nil {
						res.Skipped = append(res.Skipped, fmt.Sprintf("Zeile %d (id %d): plannedDate/Time: %v", i+2, id, err))
						continue
					}
					plannedStarttime = &when
				}

				exam := &model.PreplanExam{
					ID: id, ExamKind: cell(row, 1), ExamerID: examerID, ExamerName: cell(row, 3),
					Module: cell(row, 4), Programs: s2strs(cell(row, 5)), ExpectedStudents: expected,
					Duration: duration, PlannedStarttime: plannedStarttime,
					IsFixed: s2b(cell(row, 10)), NotSameSlot: notSame, CanShareSlot: canShare,
					Ancode: ancode, Notes: cell(row, 14),
				}
				if err := p.dbClient.UpsertPreplanExam(ctx, exam); err != nil {
					res.Skipped = append(res.Skipped, fmt.Sprintf("Zeile %d (id %d): %v", i+2, id, err))
					continue
				}
				res.Applied++
			}
			return res, nil
		},
	}
}

// ---- room requests (full replace; absolute start times) -----------------------

func (p *Plexams) csvRoomRequests() csvDataset {
	header := []string{"room", "startDate", "startTime", "fromDate", "fromTime", "untilDate", "untilTime", "approved", "active"}
	return csvDataset{
		Title:  "Raumanfragen",
		File:   "room-requests.csv",
		Header: header,
		exportRows: func(ctx context.Context) ([][]string, error) {
			reqs, err := p.dbClient.RoomRequests(ctx)
			if err != nil {
				return nil, err
			}
			rows := make([][]string, 0, len(reqs))
			for _, r := range reqs {
				startDate, startTime := "", ""
				if r.Starttime != nil {
					startDate = r.Starttime.Format(csvDateLayout)
					startTime = r.Starttime.Format("15:04")
				}
				rows = append(rows, []string{
					r.Room, startDate, startTime,
					r.From.Format(csvDateLayout), r.From.Format("15:04"),
					r.Until.Format(csvDateLayout), r.Until.Format("15:04"),
					b2s(r.Approved), b2s(r.Active),
				})
			}
			return rows, nil
		},
		importRows: func(ctx context.Context, rows [][]string) (*CSVImportResult, error) {
			res := newCSVImportResult("room-requests")
			// full replace: parse everything first, refuse an empty file (would wipe all).
			if len(rows) == 0 {
				return nil, fmt.Errorf("leere CSV (keine Zeilen) — Raumanfragen werden nicht gelöscht")
			}
			requests := make([]*model.RoomRequest, 0, len(rows))
			for i, row := range rows {
				start, err1 := time.ParseInLocation(csvDateTimeLayout, cell(row, 1)+" "+cell(row, 2), time.Local)
				from, err3 := time.ParseInLocation(csvDateTimeLayout, cell(row, 3)+" "+cell(row, 4), time.Local)
				until, err4 := time.ParseInLocation(csvDateTimeLayout, cell(row, 5)+" "+cell(row, 6), time.Local)
				if err := firstErr(err1, err3, err4); err != nil {
					return nil, fmt.Errorf("ungültige Zeile %d: %w — nichts geändert", i+2, err)
				}
				requests = append(requests, &model.RoomRequest{
					Room: cell(row, 0), Starttime: &start, From: from, Until: until,
					Approved: s2b(cell(row, 7)), Active: s2b(cell(row, 8)),
				})
			}
			if err := p.dbClient.ReplaceAllRoomRequests(ctx, requests); err != nil {
				return nil, err
			}
			res.Applied = len(requests)
			return res, nil
		},
	}
}

// ---- exam-to-plan selection ---------------------------------------------------

func (p *Plexams) csvExamsToPlan() csvDataset {
	header := []string{"ancode", "toPlan"}
	return csvDataset{
		Title:  "Prüfungsauswahl (zu planen / nicht)",
		File:   "exams-to-plan.csv",
		Header: header,
		exportRows: func(ctx context.Context) ([][]string, error) {
			toPlan, err := p.dbClient.GetZPAExamsToPlan(ctx)
			if err != nil {
				return nil, err
			}
			notToPlan, err := p.dbClient.GetZPAExamsNotToPlan(ctx)
			if err != nil {
				return nil, err
			}
			rows := make([][]string, 0, len(toPlan)+len(notToPlan))
			for _, e := range toPlan {
				rows = append(rows, []string{strconv.Itoa(e.AnCode), "true"})
			}
			for _, e := range notToPlan {
				rows = append(rows, []string{strconv.Itoa(e.AnCode), "false"})
			}
			sort.Slice(rows, func(i, j int) bool { return rows[i][0] < rows[j][0] })
			return rows, nil
		},
		importRows: func(ctx context.Context, rows [][]string) (*CSVImportResult, error) {
			res := newCSVImportResult("exams-to-plan")
			for i, row := range rows {
				ancode, err := strconv.Atoi(cell(row, 0))
				if err != nil {
					res.Skipped = append(res.Skipped, fmt.Sprintf("Zeile %d: ungültiger ancode %q", i+2, cell(row, 0)))
					continue
				}
				if s2b(cell(row, 1)) {
					_, err = p.AddZpaExamToPlan(ctx, ancode)
				} else {
					_, err = p.RmZpaExamFromPlan(ctx, ancode)
				}
				if err != nil {
					res.Skipped = append(res.Skipped, fmt.Sprintf("Zeile %d (ancode %d): %v", i+2, ancode, err))
					continue
				}
				res.Applied++
			}
			return res, nil
		},
	}
}

// ---- duration overrides -------------------------------------------------------

func (p *Plexams) csvDurationOverrides() csvDataset {
	header := []string{"ancode", "duration"}
	return csvDataset{
		Title:  "Dauer-Overrides",
		File:   "duration-overrides.csv",
		Header: header,
		exportRows: func(ctx context.Context) ([][]string, error) {
			ovs, err := p.dbClient.ExamDurationOverrides(ctx)
			if err != nil {
				return nil, err
			}
			rows := make([][]string, 0, len(ovs))
			for _, o := range ovs {
				rows = append(rows, []string{strconv.Itoa(o.Ancode), strconv.Itoa(o.Duration)})
			}
			return rows, nil
		},
		importRows: func(ctx context.Context, rows [][]string) (*CSVImportResult, error) {
			res := newCSVImportResult("duration-overrides")
			for i, row := range rows {
				ancode, err1 := strconv.Atoi(cell(row, 0))
				duration, err2 := strconv.Atoi(cell(row, 1))
				if err := firstErr(err1, err2); err != nil {
					res.Skipped = append(res.Skipped, fmt.Sprintf("Zeile %d: %v", i+2, err))
					continue
				}
				if _, err := p.dbClient.SetExamDurationOverride(ctx, ancode, duration); err != nil {
					res.Skipped = append(res.Skipped, fmt.Sprintf("Zeile %d (ancode %d): %v", i+2, ancode, err))
					continue
				}
				res.Applied++
			}
			return res, nil
		},
	}
}

// ---- conflict ratings ---------------------------------------------------------

func (p *Plexams) csvConflictRatings() csvDataset {
	header := []string{"ancode1", "ancode2", "mtknr", "decision"}
	return csvDataset{
		Title:  "Konflikt-Ratings (ACCEPT/VETO)",
		File:   "conflict-ratings.csv",
		Header: header,
		exportRows: func(ctx context.Context) ([][]string, error) {
			ds, err := p.dbClient.StudentConflictDecisions(ctx)
			if err != nil {
				return nil, err
			}
			rows := make([][]string, 0, len(ds))
			for _, d := range ds {
				rows = append(rows, []string{strconv.Itoa(d.Ancode1), strconv.Itoa(d.Ancode2), d.Mtknr, string(d.Decision)})
			}
			return rows, nil
		},
		importRows: func(ctx context.Context, rows [][]string) (*CSVImportResult, error) {
			res := newCSVImportResult("conflict-ratings")
			for i, row := range rows {
				a1, err1 := strconv.Atoi(cell(row, 0))
				a2, err2 := strconv.Atoi(cell(row, 1))
				if err := firstErr(err1, err2); err != nil {
					res.Skipped = append(res.Skipped, fmt.Sprintf("Zeile %d: %v", i+2, err))
					continue
				}
				decision := strings.ToUpper(cell(row, 3))
				if decision != string(model.ConflictDecisionAccept) && decision != string(model.ConflictDecisionVeto) {
					res.Skipped = append(res.Skipped, fmt.Sprintf("Zeile %d: ungültige decision %q (ACCEPT/VETO)", i+2, cell(row, 3)))
					continue
				}
				if err := p.dbClient.UpsertDecision(ctx, a1, a2, cell(row, 2), decision); err != nil {
					res.Skipped = append(res.Skipped, fmt.Sprintf("Zeile %d: %v", i+2, err))
					continue
				}
				res.Applied++
			}
			return res, nil
		},
	}
}

// ---- can share slot -----------------------------------------------------------

func (p *Plexams) csvCanShareSlot() csvDataset {
	header := []string{"ancode1", "ancode2"}
	return csvDataset{
		Title:  "Dürfen sich einen Slot teilen",
		File:   "can-share-slot.csv",
		Header: header,
		exportRows: func(ctx context.Context) ([][]string, error) {
			pairs, err := p.dbClient.CanShareSlotPairs(ctx)
			if err != nil {
				return nil, err
			}
			rows := make([][]string, 0, len(pairs))
			for _, pr := range pairs {
				rows = append(rows, []string{strconv.Itoa(pr[0]), strconv.Itoa(pr[1])})
			}
			return rows, nil
		},
		importRows: func(ctx context.Context, rows [][]string) (*CSVImportResult, error) {
			res := newCSVImportResult("can-share-slot")
			for i, row := range rows {
				a1, err1 := strconv.Atoi(cell(row, 0))
				a2, err2 := strconv.Atoi(cell(row, 1))
				if err := firstErr(err1, err2); err != nil {
					res.Skipped = append(res.Skipped, fmt.Sprintf("Zeile %d: %v", i+2, err))
					continue
				}
				if err := p.dbClient.UpsertCanShareSlot(ctx, a1, a2); err != nil {
					res.Skipped = append(res.Skipped, fmt.Sprintf("Zeile %d: %v", i+2, err))
					continue
				}
				res.Applied++
			}
			return res, nil
		},
	}
}

func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

// ---- top-level export/import + zip --------------------------------------------

// DatasetCSV builds the CSV of one dataset and a suggested download filename.
func (p *Plexams) DatasetCSV(ctx context.Context, name string) ([]byte, string, error) {
	ds, ok := p.csvDatasets()[name]
	if !ok {
		return nil, "", fmt.Errorf("unknown csv dataset %q", name)
	}
	rows, err := ds.exportRows(ctx)
	if err != nil {
		return nil, "", err
	}
	data, err := encodeCSV(ds.Header, rows)
	if err != nil {
		return nil, "", err
	}
	filename := fmt.Sprintf("%s_%s", strings.ReplaceAll(p.dbClient.DatabaseName(), " ", "_"), ds.File)
	return data, filename, nil
}

// ImportDatasetCSV applies one dataset CSV to the current database.
func (p *Plexams) ImportDatasetCSV(ctx context.Context, name string, data []byte) (*CSVImportResult, error) {
	ds, ok := p.csvDatasets()[name]
	if !ok {
		return nil, fmt.Errorf("unknown csv dataset %q", name)
	}
	rows, err := decodeCSV(data, ds.Header)
	if err != nil {
		return nil, err
	}
	res, err := ds.importRows(ctx, rows)
	if err != nil {
		return nil, err
	}
	log.Info().Str("dataset", name).Int("applied", res.Applied).Int("skipped", len(res.Skipped)).Msg("imported dataset csv")
	return res, nil
}

// MyInputsCSVZip builds a ZIP with one CSV per dataset (everything the planner entered).
func (p *Plexams) MyInputsCSVZip(ctx context.Context) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	datasets := p.csvDatasets()
	for _, name := range csvDatasetOrder {
		ds := datasets[name]
		rows, err := ds.exportRows(ctx)
		if err != nil {
			return nil, fmt.Errorf("dataset %q: %w", name, err)
		}
		data, err := encodeCSV(ds.Header, rows)
		if err != nil {
			return nil, err
		}
		f, err := zw.Create(ds.File)
		if err != nil {
			return nil, err
		}
		if _, err := f.Write(data); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ---- HTTP handlers ------------------------------------------------------------

// HTTPDownloadDatasetCSV streams one dataset as a CSV download.
// GET /download/dataset-csv?name=<dataset>
func (p *Plexams) HTTPDownloadDatasetCSV(w http.ResponseWriter, r *http.Request) {
	data, filename, err := p.DatasetCSV(r.Context(), r.URL.Query().Get("name"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	if _, err := w.Write(data); err != nil {
		log.Error().Err(err).Msg("cannot write dataset csv download")
	}
}

// HTTPDownloadMyInputsCSV streams the combined CSV ZIP of all entered data.
// GET /download/my-inputs-csv.zip
func (p *Plexams) HTTPDownloadMyInputsCSV(w http.ResponseWriter, r *http.Request) {
	data, err := p.MyInputsCSVZip(r.Context())
	if err != nil {
		http.Error(w, "cannot build csv zip: "+err.Error(), http.StatusInternalServerError)
		return
	}
	filename := fmt.Sprintf("%s_meine-eingaben-csv.zip", strings.ReplaceAll(p.dbClient.DatabaseName(), " ", "_"))
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	if _, err := w.Write(data); err != nil {
		log.Error().Err(err).Msg("cannot write csv zip download")
	}
}

// HTTPUploadDatasetCSV imports one dataset CSV into the current database.
// POST /upload/dataset-csv  (multipart form: name=<dataset>, file=<csv>)
func (p *Plexams) HTTPUploadDatasetCSV(w http.ResponseWriter, r *http.Request) {
	if !p.WritesAllowed() {
		http.Error(w, "a validation or transfer/email is running, cannot upload now", http.StatusConflict)
		return
	}
	if p.IsReadOnly() {
		http.Error(w, "semester is read-only", http.StatusConflict)
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		http.Error(w, "cannot parse upload: "+err.Error(), http.StatusBadRequest)
		return
	}
	name := r.FormValue("name")
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close() //nolint:errcheck

	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "cannot read file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	result, err := p.ImportDatasetCSV(r.Context(), name, data)
	if err != nil {
		http.Error(w, "import failed: "+err.Error(), http.StatusBadRequest)
		return
	}
	p.LogUpload(r.Context(), "uploadDatasetCSV", "dataset", name)
	writeJSON(w, result)
}
