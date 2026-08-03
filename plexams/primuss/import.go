package primuss

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/obcode/plexams.go/db"
	"github.com/xuri/excelize/v2"
)

// ImportResult summarizes a Primuss XLSX ZIP import.
type ImportResult struct {
	Programs []*ImportProgram
	Skipped  []string // files in the zip that were ignored
}

// ImportProgram is the per-program outcome.
type ImportProgram struct {
	Program        string
	ExamsImported  int
	StudentRegs    int
	CountRows      int
	ConflictRows   int
	Missing        []string // file types not present for this program
	FirstImport    bool     // no prior studentregs for this program (initial import, not an update)
	ChangedAncodes []int    // ancodes whose registrations changed vs before (empty on first import)
	// Problems are the rows and cells the converters could not use. They used to
	// be silent: an unparseable ancode became 0 with a log.Debug for company.
	Problems []string
}

// primussGroupRE extracts the degree-suffixed program code from a Sammellisten
// filename, e.g. "Prüfungsanmeldungen-IF-B-126.xlsx" -> "IF-B". Keeping the B/M
// degree marker in the code makes Bachelor and Master of the same 2-letter code
// (e.g. DC-B vs DC-M) distinct programs / collections instead of colliding in a
// single "DC".
var primussGroupRE = regexp.MustCompile(`-([A-Z]{2,4}-[BM])-`)

// detectPrimussFile returns the program and the collection kind (studentregs | exams |
// count | conflicts) for a Sammellisten filename, or empty kind if it is not one of the
// four imported file types.
func detectPrimussFile(base string) (program, kind string) {
	m := primussGroupRE.FindStringSubmatch(base)
	if m == nil {
		return "", ""
	}
	program = m[1]
	lower := strings.ToLower(base)
	switch {
	case strings.Contains(lower, "anmeldungen"):
		kind = "studentregs"
	case strings.Contains(lower, "katalog"):
		kind = "exams"
	case strings.Contains(lower, "planung"):
		kind = "count"
	case strings.Contains(lower, "nach_ancode"):
		kind = "conflicts"
	default:
		kind = "" // e.g. the CodeNr-keyed "Prüfungsüberschneidungen" — ignored
	}
	return program, kind
}

// ImportZip imports the Primuss XLSX files from an uploaded ZIP. The program is derived
// from each filename; only the four known file types are imported (drop+insert per
// program). Only the programs/collections actually present in the ZIP are touched
// (incremental). For each replaced studentregs collection it reports the ancodes whose
// registrations changed, so update emails can be sent to those examers.
func (s *Service) ImportZip(ctx context.Context, zipData []byte) (*ImportResult, error) {
	zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, fmt.Errorf("not a valid zip: %w", err)
	}

	// program -> kind -> xlsx bytes (last one wins)
	files := make(map[string]map[string][]byte)
	result := &ImportResult{Skipped: []string{}}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		base := filepath.Base(f.Name)
		if strings.HasPrefix(base, ".") || strings.HasPrefix(f.Name, "__MACOSX") {
			continue
		}
		program, kind := detectPrimussFile(base)
		if kind == "" {
			result.Skipped = append(result.Skipped, base)
			continue
		}
		rc, err := f.Open()
		if err != nil {
			result.Skipped = append(result.Skipped, base+" (cannot open)")
			continue
		}
		data, err := io.ReadAll(rc)
		rc.Close() //nolint:errcheck
		if err != nil {
			result.Skipped = append(result.Skipped, base+" (cannot read)")
			continue
		}
		if files[program] == nil {
			files[program] = make(map[string][]byte)
		}
		files[program][kind] = data
	}

	programs := make([]string, 0, len(files))
	for program := range files {
		programs = append(programs, program)
	}
	sort.Strings(programs)

	for _, program := range programs {
		prog, err := s.importProgram(ctx, program, files[program])
		if err != nil {
			return nil, fmt.Errorf("program %s: %w", program, err)
		}
		result.Programs = append(result.Programs, prog)
	}
	return result, nil
}

// ImportDir zips all .xlsx under dir (recursively) in memory and imports them like an
// uploaded ZIP. Convenience for the CLI / a server-side directory.
func (s *Service) ImportDir(ctx context.Context, dir string) (*ImportResult, error) {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.EqualFold(filepath.Ext(path), ".xlsx") {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		w, err := zw.Create(rel)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	})
	if err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return s.ImportZip(ctx, buf.Bytes())
}

func (s *Service) importProgram(ctx context.Context, program string, byKind map[string][]byte) (*ImportProgram, error) {
	res := &ImportProgram{Program: program, ChangedAncodes: []int{}, Missing: []string{}, Problems: []string{}}

	addProblems := func(errs []ConvertError) {
		for _, e := range errs {
			res.Problems = append(res.Problems, e.String())
		}
	}

	// The exam catalogue goes first: the counts and the conflicts reference it,
	// and with foreign keys that order is no longer a matter of taste.
	if data, ok := byKind["exams"]; ok {
		sh, err := parseSheet(data)
		if err != nil {
			return nil, fmt.Errorf("exams: %w", err)
		}
		rows, errs := convertExams(sh)
		addProblems(errs)
		n, err := s.db.ReplacePrimussExams(ctx, program, rows)
		if err != nil {
			return nil, err
		}
		res.ExamsImported = n
	} else {
		res.Missing = append(res.Missing, "exams")
	}

	if data, ok := byKind["studentregs"]; ok {
		old, err := s.db.PrimussStudentRegRows(ctx, program)
		if err != nil {
			return nil, err
		}
		sh, err := parseSheet(data)
		if err != nil {
			return nil, fmt.Errorf("studentregs: %w", err)
		}
		rows, errs := convertStudentRegs(sh)
		addProblems(errs)

		// changed ancodes only make sense as an update against prior data; the first
		// import of a program is the initial data, not an update.
		if len(old) == 0 {
			res.FirstImport = true
		} else {
			res.ChangedAncodes = changedAncodes(old, rows)
		}

		n, err := s.db.ReplacePrimussStudentRegs(ctx, program, rows)
		if err != nil {
			return nil, err
		}
		res.StudentRegs = n
	} else {
		res.Missing = append(res.Missing, "studentregs")
	}

	if data, ok := byKind["count"]; ok {
		sh, err := parseSheet(data)
		if err != nil {
			return nil, fmt.Errorf("count: %w", err)
		}
		rows, errs := convertCounts(sh)
		addProblems(errs)
		n, err := s.db.ReplacePrimussCounts(ctx, program, rows)
		if err != nil {
			return nil, err
		}
		res.CountRows = n
	} else {
		res.Missing = append(res.Missing, "count")
	}

	if data, ok := byKind["conflicts"]; ok {
		sh, err := parseSheet(data)
		if err != nil {
			return nil, fmt.Errorf("conflicts: %w", err)
		}
		rows, errs := convertConflicts(sh)
		addProblems(errs)

		// A conflict may only name exams the catalogue of this program has. The
		// foreign keys would reject the whole write; dropping and reporting turns
		// that into a line the planner can act on.
		if examRows, ok := byKind["exams"]; ok {
			known, err := knownAncodes(examRows)
			if err != nil {
				return nil, fmt.Errorf("conflicts: %w", err)
			}
			var dropped []ConvertError
			rows, dropped = dropConflictsWithUnknownExams(rows, known)
			addProblems(dropped)
		}

		n, err := s.db.ReplacePrimussConflicts(ctx, program, rows)
		if err != nil {
			return nil, err
		}
		res.ConflictRows = n
	} else {
		res.Missing = append(res.Missing, "conflicts")
	}

	return res, nil
}

// knownAncodes is the set of ancodes in a program's exam catalogue.
func knownAncodes(examData []byte) (map[int]bool, error) {
	sh, err := parseSheet(examData)
	if err != nil {
		return nil, err
	}
	rows, _ := convertExams(sh)
	known := make(map[int]bool, len(rows))
	for _, row := range rows {
		known[row.Ancode] = true
	}
	return known, nil
}

// xlsxRows opens an in-memory xlsx and returns the rows of its first sheet, each padded
// to the header length.
func xlsxRows(data []byte) ([][]string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("xlsx has no sheet")
	}
	rows, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, err
	}
	if len(rows) < 1 {
		return nil, fmt.Errorf("xlsx is empty")
	}
	width := len(rows[0])
	for i, r := range rows {
		for len(r) < width {
			r = append(r, "")
		}
		rows[i] = r
	}
	return rows, nil
}

// parseSheet reads the first sheet of an in-memory xlsx into a trimmed header
// plus its rows, every row padded to the header's width.
func parseSheet(data []byte) (*sheet, error) {
	rows, err := xlsxRows(data)
	if err != nil {
		return nil, err
	}
	header := make([]string, len(rows[0]))
	for i, h := range rows[0] {
		header[i] = strings.TrimSpace(h)
	}
	return &sheet{header: header, rows: rows[1:]}, nil
}

// changedAncodes compares the old and new registrations and returns the ancodes
// whose registration set changed (added/removed students or changed fields).
func changedAncodes(oldRows, newRows []db.PrimussStudentRegRow) []int {
	oldSig := studentregSignatures(oldRows)
	newSig := studentregSignatures(newRows)
	changedSet := make(map[int]bool)
	for ancode, sig := range newSig {
		if oldSig[ancode] != sig {
			changedSet[ancode] = true
		}
	}
	for ancode := range oldSig {
		if _, ok := newSig[ancode]; !ok {
			changedSet[ancode] = true
		}
	}
	changed := make([]int, 0, len(changedSet))
	for ancode := range changedSet {
		changed = append(changed, ancode)
	}
	sort.Ints(changed)
	return changed
}

// studentregSignatures builds, per ancode, a stable signature of its
// registrations.
//
// Note, gebucht and nicht_zul are not modelled columns, so they come out of Raw
// -- they are exactly the fields that say a registration changed without the
// student changing, and leaving them out would make a re-import look quiet when
// it is not.
func studentregSignatures(rows []db.PrimussStudentRegRow) map[int]string {
	rowsByAncode := make(map[int][]string)
	for _, r := range rows {
		line := fmt.Sprintf("%v|%v|%v|%v|%v",
			r.Mtknr, rawValue(r.Raw, "Note"), r.Group,
			rawValue(r.Raw, "gebucht"), rawValue(r.Raw, "nicht_zul"))
		rowsByAncode[r.PrimussAncode] = append(rowsByAncode[r.PrimussAncode], line)
	}
	sigs := make(map[int]string, len(rowsByAncode))
	for ancode, lines := range rowsByAncode {
		sort.Strings(lines)
		sigs[ancode] = strings.Join(lines, "\n")
	}
	return sigs
}

// rawValue renders an unmodelled column for the signature. A missing column and
// an empty one have to look the same, or a Primuss export that stops sending a
// column would mark every exam as changed.
func rawValue(raw map[string]any, key string) string {
	v, ok := raw[key]
	if !ok || v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}
