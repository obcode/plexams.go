// Package joint holds the pure MUC.DAI CSV import parsing: turning a raw CSV export (of
// varying encoding and delimiter) into db.JointExam records grouped by program. The
// plexams package does the surrounding I/O (replacing collections, generating exams); this
// parser is I/O-free and unit-tested (mirrors plexams/primuss).
package joint

import (
	"encoding/csv"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/obcode/plexams.go/db"
	"github.com/rs/zerolog/log"
)

// columns maps the (normalized) CSV header names to the db.JointExam fields.
var columns = map[string]string{
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

// ParseCSV parses the CSV text (comma/semicolon/tab auto-detected, ISO-8859-1 tolerated)
// into db.JointExam grouped by program (Studiengruppe). Rows without a numeric Nr or a
// program are skipped; the file must have the 'Nr' and 'Studiengruppe' columns.
func ParseCSV(csvText string) (map[string][]*db.JointExam, error) {
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
	// NB: do NOT set TrimLeadingSpace — with a whitespace delimiter (tab) it collapses
	// empty fields between two tabs, shifting all later columns. Field values are
	// trimmed in get() anyway.

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
		key := normalizeHeader(h)
		if field, ok := columns[key]; ok {
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

	byProgram := make(map[string][]*db.JointExam)
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
		byProgram[program] = append(byProgram[program], &db.JointExam{
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

// headerTypeSuffix matches a mongoimport type suffix, e.g. ".int32()" in "Nr.int32()" or
// ".string()" in "Modulname.string()".
var headerTypeSuffix = regexp.MustCompile(`\.\w+\(\)\s*$`)

// normalizeHeader lowercases a header and strips a mongoimport type suffix, so both "Nr"
// and "Nr.int32()" map to "nr".
func normalizeHeader(h string) string {
	h = headerTypeSuffix.ReplaceAllString(strings.TrimSpace(h), "")
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
