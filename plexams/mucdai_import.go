package plexams

import (
	"context"
	"encoding/csv"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// externalAncodeBase is the lower bound for auto-assigned ancodes of external
// (e.g. MUC.DAI) exams. Assigned ancodes are >= this and above any existing one, so
// they never collide and stay >= 1000 (the non-ZPA marker).
const externalAncodeBase = 90000

// mucDaiPlannerFK07 marks exams planned by FK07 themselves: those already exist as
// ZPA exams and are only linked, not generated.
const mucDaiPlannerFK07 = "FK07"

// ImportMucDaiExams parses a MUC.DAI CSV, replaces the mucdai_<program> collections
// and generates the non-ZPA exams for all exams not planned by FK07 (assigning a
// stable ancode to new ones). FK07 exams are left to the normal ZPA flow.
func (p *Plexams) ImportMucDaiExams(ctx context.Context, csvText string) (*model.ImportMucDaiResult, error) {
	byProgram, err := parseMucDaiCSV(csvText)
	if err != nil {
		return nil, err
	}
	if len(byProgram) == 0 {
		return nil, fmt.Errorf("no MUC.DAI exams found in CSV (check the column headers)")
	}

	result := &model.ImportMucDaiResult{Programs: []string{}}

	programs := make([]string, 0, len(byProgram))
	for program := range byProgram {
		programs = append(programs, program)
	}
	sort.Strings(programs)

	for _, program := range programs {
		exams := byProgram[program]
		if err := p.dbClient.ReplaceMucDaiExamsForProgram(ctx, program, exams); err != nil {
			return nil, err
		}
		result.Programs = append(result.Programs, program)
		result.ExamsImported += len(exams)
	}

	// generate the non-ZPA exams for non-FK07 exams
	existing, maxAncode, err := p.existingNonZpaByPrimuss(ctx)
	if err != nil {
		return nil, err
	}
	nextAncode := externalAncodeBase
	if maxAncode >= nextAncode {
		nextAncode = maxAncode + 1
	}

	importedPrograms := make(map[string]bool, len(programs))
	for _, program := range programs {
		importedPrograms[program] = true
	}
	// keys that should have a generated exam after this import (non-FK07)
	validKeys := make(map[primussKey]bool)

	for _, program := range programs {
		modelExams, err := p.MucDaiExamsForProgram(ctx, program)
		if err != nil {
			return nil, err
		}
		for _, exam := range modelExams {
			if strings.EqualFold(strings.TrimSpace(exam.PlannedBy), mucDaiPlannerFK07) {
				result.ExamsSkippedFk07++
				continue
			}
			key := primussKey{exam.Program, exam.PrimussAncode}
			validKeys[key] = true
			if _, ok := existing[key]; ok {
				result.ExamsExisting++
				continue
			}
			if _, err := p.AddMucDaiExam(ctx, nextAncode, exam); err != nil {
				log.Error().Err(err).Str("program", exam.Program).Int("primussAncode", exam.PrimussAncode).
					Msg("cannot create mucdai exam")
				return nil, err
			}
			existing[key] = nextAncode
			result.ExamsCreated++
			nextAncode++
		}
	}

	// remove generated exams of the imported programs that are no longer in the CSV
	// (or flipped to FK07): drop the non-ZPA exam and any plan entry.
	for key, ancode := range existing {
		if !importedPrograms[key.program] || validKeys[key] {
			continue
		}
		if err := p.dbClient.DeleteNonZpaExam(ctx, ancode); err != nil {
			return nil, err
		}
		if err := p.dbClient.RemovePlanEntry(ctx, ancode); err != nil {
			return nil, err
		}
		result.ExamsRemoved++
	}

	return result, nil
}

type primussKey struct {
	program string
	ancode  int
}

// existingNonZpaByPrimuss maps (program, primussAncode) of all non-ZPA exams to their
// assigned ancode and returns the highest assigned ancode.
func (p *Plexams) existingNonZpaByPrimuss(ctx context.Context) (map[primussKey]int, int, error) {
	nonZpa, err := p.dbClient.NonZpaExams(ctx)
	if err != nil {
		return nil, 0, err
	}
	m := make(map[primussKey]int, len(nonZpa))
	maxAncode := 0
	for _, exam := range nonZpa {
		if exam.AnCode > maxAncode {
			maxAncode = exam.AnCode
		}
		for _, pa := range exam.PrimussAncodes {
			m[primussKey{pa.Program, pa.Ancode}] = exam.AnCode
		}
	}
	return m, maxAncode, nil
}

// mucDaiColumns maps the (normalized) CSV header names to the db.MucDaiExam fields.
var mucDaiColumns = map[string]string{
	"nr":               "ancode",
	"modulname":        "module",
	"prüfungsform":     "examType",
	"pruefungsform":    "examType",
	"bewertung":        "grading",
	"dauer":            "duration",
	"erstpruefender":   "mainExamer",
	"erstprüfender":    "mainExamer",
	"zweitpruefender":  "secondExamer",
	"zweitprüfender":   "secondExamer",
	"istwiederholung":  "isRepeater",
	"studiengruppe":    "program",
	"prüfungsplanung":  "planer",
	"pruefungsplanung": "planer",
}

// parseMucDaiCSV parses the CSV text (comma/semicolon/tab auto-detected) into
// db.MucDaiExam grouped by program (Studiengruppe).
func parseMucDaiCSV(csvText string) (map[string][]*db.MucDaiExam, error) {
	// MUC.DAI files are often ISO-8859-1; decode to UTF-8 if not already valid.
	if !utf8.ValidString(csvText) {
		csvText = latin1ToUTF8(csvText)
	}
	csvText = strings.TrimPrefix(csvText, "\ufeff") // strip BOM
	firstLine := csvText
	if i := strings.IndexAny(csvText, "\r\n"); i >= 0 {
		firstLine = csvText[:i]
	}
	delim := detectDelimiter(firstLine)

	reader := csv.NewReader(strings.NewReader(csvText))
	reader.Comma = delim
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true

	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("cannot parse CSV: %w", err)
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("CSV has no data rows")
	}

	// header -> column index (tolerating mongoimport type suffixes like ".int32()")
	field2col := make(map[string]int)
	for i, h := range rows[0] {
		key := normalizeMucDaiHeader(h)
		if field, ok := mucDaiColumns[key]; ok {
			field2col[field] = i
		}
	}
	if _, ok := field2col["ancode"]; !ok {
		return nil, fmt.Errorf("CSV is missing the 'Nr' column")
	}
	if _, ok := field2col["program"]; !ok {
		return nil, fmt.Errorf("CSV is missing the 'Studiengruppe' column")
	}

	get := func(row []string, field string) string {
		if col, ok := field2col[field]; ok && col < len(row) {
			return strings.TrimSpace(row[col])
		}
		return ""
	}

	byProgram := make(map[string][]*db.MucDaiExam)
	for _, row := range rows[1:] {
		ancodeStr := get(row, "ancode")
		program := get(row, "program")
		if ancodeStr == "" || program == "" {
			continue
		}
		ancode, err := strconv.Atoi(ancodeStr)
		if err != nil {
			log.Debug().Str("nr", ancodeStr).Msg("skipping MUC.DAI row with non-numeric Nr")
			continue
		}
		duration, _ := strconv.Atoi(get(row, "duration"))
		byProgram[program] = append(byProgram[program], &db.MucDaiExam{
			PrimussAncode:  ancode,
			Module:         get(row, "module"),
			ExamType:       get(row, "examType"),
			Grading:        get(row, "grading"),
			Duration:       duration,
			MainExamer:     get(row, "mainExamer"),
			SecondExamer:   get(row, "secondExamer"),
			IsRepeaterExam: get(row, "isRepeater"),
			Program:        program,
			Planer:         get(row, "planer"),
		})
	}
	return byProgram, nil
}

// mucDaiHeaderTypeSuffix matches a mongoimport type suffix, e.g. ".int32()" in
// "Nr.int32()" or ".string()" in "Modulname.string()".
var mucDaiHeaderTypeSuffix = regexp.MustCompile(`\.\w+\(\)\s*$`)

// normalizeMucDaiHeader lowercases a header and strips a mongoimport type suffix, so
// both "Nr" and "Nr.int32()" map to "nr".
func normalizeMucDaiHeader(h string) string {
	h = mucDaiHeaderTypeSuffix.ReplaceAllString(strings.TrimSpace(h), "")
	return strings.ToLower(strings.TrimSpace(h))
}

// latin1ToUTF8 decodes ISO-8859-1 bytes to a UTF-8 string (each byte is a code point).
func latin1ToUTF8(s string) string {
	runes := make([]rune, 0, len(s))
	for _, b := range []byte(s) {
		runes = append(runes, rune(b))
	}
	return string(runes)
}

// detectDelimiter picks the most frequent of ';' '\t' ',' in the header line.
func detectDelimiter(headerLine string) rune {
	semicolons := strings.Count(headerLine, ";")
	tabs := strings.Count(headerLine, "\t")
	commas := strings.Count(headerLine, ",")
	switch {
	case semicolons >= commas && semicolons >= tabs && semicolons > 0:
		return ';'
	case tabs >= commas && tabs > 0:
		return '\t'
	default:
		return ','
	}
}
