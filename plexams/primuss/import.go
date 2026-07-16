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
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
	"go.mongodb.org/mongo-driver/bson"
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
}

// primussGroupRE extracts the degree-suffixed program code from a Sammellisten
// filename, e.g. "Prüfungsanmeldungen-IF-B-126.xlsx" -> "IF-B". Keeping the B/M
// degree marker in the code makes Bachelor and Master of the same 2-letter code
// (e.g. DC-B vs DC-M) distinct programs / collections instead of colliding in a
// single "DC".
var primussGroupRE = regexp.MustCompile(`-([A-Z]{2,4}-[BM])-`)

// primussStudentregNumeric marks the numeric columns of the Prüfungsanmeldungen file.
// Only AnCode is numeric; everything else (incl. MTKNR) stays a string.
var primussStudentregNumeric = map[string]bool{"AnCode": true}

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
	res := &ImportProgram{Program: program, ChangedAncodes: []int{}, Missing: []string{}}

	// studentregs first (with change detection against the existing collection)
	if data, ok := byKind["studentregs"]; ok {
		old, err := s.db.RawCollection(ctx, "studentregs_"+program)
		if err != nil {
			return nil, err
		}
		docs, err := parsePrimussStudentregs(data)
		if err != nil {
			return nil, fmt.Errorf("studentregs: %w", err)
		}
		// changed ancodes only make sense as an update against prior data; the first
		// import of a program is the initial data, not an update.
		if len(old) == 0 {
			res.FirstImport = true
		} else {
			res.ChangedAncodes = changedAncodes(old, docs)
		}
		n, err := s.db.ReplaceRawCollection(ctx, "studentregs_"+program, docs)
		if err != nil {
			return nil, err
		}
		res.StudentRegs = n
	} else {
		res.Missing = append(res.Missing, "studentregs")
	}

	imports := []struct {
		kind, collection     string
		sumFix, ignoreBlanks bool
		set                  *int
	}{
		{"exams", "exams_" + program, false, false, &res.ExamsImported},
		{"count", "count_" + program, true, false, &res.CountRows},
		{"conflicts", "conflicts_" + program, false, true, &res.ConflictRows},
	}
	for _, imp := range imports {
		data, ok := byKind[imp.kind]
		if !ok {
			res.Missing = append(res.Missing, imp.kind)
			continue
		}
		docs, err := parsePrimussAutoTyped(data, imp.sumFix, imp.ignoreBlanks)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", imp.kind, err)
		}
		n, err := s.db.ReplaceRawCollection(ctx, imp.collection, docs)
		if err != nil {
			return nil, err
		}
		*imp.set = n
	}
	return res, nil
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

// parsePrimussStudentregs maps the Prüfungsanmeldungen rows to docs with the fixed typing
// (AnCode int, everything else — incl. MTKNR — string).
func parsePrimussStudentregs(data []byte) ([]bson.M, error) {
	rows, err := xlsxRows(data)
	if err != nil {
		return nil, err
	}
	header := trimmedHeader(rows[0], false)
	docs := make([]bson.M, 0, len(rows)-1)
	for _, row := range rows[1:] {
		doc := bson.M{}
		for i, name := range header {
			if name == "" {
				continue
			}
			val := strings.TrimSpace(row[i])
			if primussStudentregNumeric[name] {
				if n, err := strconv.Atoi(val); err == nil {
					doc[name] = n
					continue
				}
			}
			doc[name] = val
		}
		docs = append(docs, doc)
	}
	return docs, nil
}

// parsePrimussAutoTyped maps rows to docs with mongoimport-like auto typing: integer cells
// become ints, others strings. With sumFix the header "Sum." becomes "Sum"; with
// ignoreBlanks empty cells are omitted (else kept as "").
func parsePrimussAutoTyped(data []byte, sumFix, ignoreBlanks bool) ([]bson.M, error) {
	rows, err := xlsxRows(data)
	if err != nil {
		return nil, err
	}
	header := trimmedHeader(rows[0], sumFix)
	docs := make([]bson.M, 0, len(rows)-1)
	for _, row := range rows[1:] {
		doc := bson.M{}
		for i, name := range header {
			if name == "" {
				continue
			}
			val := strings.TrimSpace(row[i])
			if val == "" {
				if !ignoreBlanks {
					doc[name] = ""
				}
				continue
			}
			if n, err := strconv.Atoi(val); err == nil {
				doc[name] = n
			} else {
				doc[name] = val
			}
		}
		docs = append(docs, doc)
	}
	return docs, nil
}

func trimmedHeader(row []string, sumFix bool) []string {
	header := make([]string, len(row))
	for i, h := range row {
		h = strings.TrimSpace(h)
		if sumFix && h == "Sum." {
			h = "Sum"
		}
		header[i] = h
	}
	return header
}

// changedAncodes compares the old and new studentreg docs and returns the ancodes whose
// registration set changed (added/removed students or changed registration fields).
func changedAncodes(oldDocs, newDocs []bson.M) []int {
	oldSig := studentregSignatures(oldDocs)
	newSig := studentregSignatures(newDocs)
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

// studentregSignatures builds, per ancode, a stable signature of its registrations.
func studentregSignatures(docs []bson.M) map[int]string {
	rowsByAncode := make(map[int][]string)
	for _, d := range docs {
		ancode := toInt(d["AnCode"])
		row := fmt.Sprintf("%v|%v|%v|%v|%v",
			d["MTKNR"], d["Note"], d["Stgru"], d["gebucht"], d["nicht_zul"])
		rowsByAncode[ancode] = append(rowsByAncode[ancode], row)
	}
	sigs := make(map[int]string, len(rowsByAncode))
	for ancode, rows := range rowsByAncode {
		sort.Strings(rows)
		sigs[ancode] = strings.Join(rows, "\n")
	}
	return sigs
}

func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	case float64:
		return int(n)
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(n))
		return i
	default:
		return 0
	}
}
