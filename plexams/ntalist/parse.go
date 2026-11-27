package ntalist

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

// List is one parsed NTA list, i.e. one faculty in one semester.
type List struct {
	Filename string
	// FK is the faculty number with two digits ("07", "21").
	FK string
	// SemesterCode is the PRIMUSS code ("226"), SemesterID the plexams id ("2026-WS").
	SemesterCode string
	SemesterID   string
	// Stand is the time the list was exported; zero if neither the sheet nor the
	// file name says.
	Stand    time.Time
	Rows     []*Row
	Warnings []string
}

// Row is one student of the list.
type Row struct {
	// Line is the row number in the sheet (1-based), for messages.
	Line          int    `json:"line"`
	FK            string `json:"fk"`
	Stg           string `json:"stg"`
	Mtknr         string `json:"mtknr"`
	Vorname       string `json:"vname"`
	Name          string `json:"name"`
	BeginnStudium string `json:"beginnStudium"`
	// Compensation is the cleaned free text of the column nachteilsausgleich.
	Compensation string       `json:"compensation"`
	Parsed       Compensation `json:"parsed"`
	// Validity is the raw semester column ("*", "226:127"), Until its conversion.
	Validity string   `json:"validity"`
	Until    string   `json:"until"`
	Regs     []Reg    `json:"regs"`
	Problems []string `json:"problems"`
}

// Reg is one registration of the column anmeldungen_aktsemester, e.g. "IF-B:123".
type Reg struct {
	Stg           string `json:"stg"`
	PrimussAncode int    `json:"primussAncode"`
}

var (
	// "226_07_Liste_NTA_Stand_27-11-26.xlsx"; a prefix (e.g. from a download) is
	// tolerated.
	filenameRE  = regexp.MustCompile(`(?i)(\d{3})_(\d{1,2})_Liste_NTA_Stand_(\d{2})-(\d{2})-(\d{2})`)
	fkCellRE    = regexp.MustCompile(`^FK\s*(\d{1,2})$`)
	semCellRE   = regexp.MustCompile(`^für Semester\s+(\d{3})$`)
	standCellRE = regexp.MustCompile(`^\d{2}\.\d{2}\.\d{4}\s+\d{2}:\d{2}(:\d{2})?$`)
)

var requiredColumns = []string{"fk", "stg", "mtknr", "vname", "name", "beginn_studium",
	"nachteilsausgleich", "semester", "anmeldungen_aktsemester"}

func normFK(fk string) string {
	n, err := strconv.Atoi(strings.TrimSpace(fk))
	if err != nil {
		return strings.TrimSpace(fk)
	}
	return fmt.Sprintf("%02d", n)
}

// Parse reads one NTA list. An error means the file cannot be used at all;
// problems of single rows are reported on the row.
func Parse(filename string, data []byte) (*List, error) {
	if !strings.EqualFold(filepath.Ext(filename), ".xlsx") {
		return nil, fmt.Errorf("%s ist keine XLSX-Datei — bitte die Excel-Liste aus dem SharePoint-Ordner verwenden", filename)
	}
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%s lässt sich nicht als Excel-Datei lesen: %w", filename, err)
	}
	defer f.Close() //nolint:errcheck
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("%s enthält kein Tabellenblatt", filename)
	}
	rows, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, fmt.Errorf("%s: das Tabellenblatt lässt sich nicht lesen: %w", filename, err)
	}
	return parseRows(filepath.Base(filename), rows)
}

func parseRows(filename string, rows [][]string) (*List, error) {
	list := &List{Filename: filename, Rows: []*Row{}, Warnings: []string{}}

	// The header row is the one that names the columns; everything above it is
	// the header block with faculty, semester and export time.
	headerIdx := -1
	cols := map[string]int{}
	for i, r := range rows {
		found := map[string]int{}
		for j, c := range r {
			found[strings.ToLower(strings.TrimSpace(c))] = j
		}
		if _, ok := found["mtknr"]; ok {
			if _, ok := found["nachteilsausgleich"]; ok {
				headerIdx, cols = i, found
				break
			}
		}
	}
	if headerIdx < 0 {
		return nil, fmt.Errorf("%s: keine Kopfzeile mit den Spalten mtknr und nachteilsausgleich gefunden — ist das die NTA-Liste?", filename)
	}
	var missing []string
	for _, c := range requiredColumns {
		if _, ok := cols[c]; !ok {
			missing = append(missing, c)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("%s: in der Kopfzeile fehlen die Spalten %s", filename, strings.Join(missing, ", "))
	}

	var cellFK, cellSem string
	var cellStand time.Time
	for _, r := range rows[:headerIdx] {
		for _, c := range r {
			c = strings.TrimSpace(c)
			if m := fkCellRE.FindStringSubmatch(c); m != nil {
				cellFK = normFK(m[1])
			} else if m := semCellRE.FindStringSubmatch(c); m != nil {
				cellSem = m[1]
			} else if standCellRE.MatchString(c) {
				layout := "02.01.2006 15:04:05"
				if strings.Count(c, ":") == 1 {
					layout = "02.01.2006 15:04"
				}
				if t, err := time.ParseInLocation(layout, strings.Join(strings.Fields(c), " "), time.Local); err == nil {
					cellStand = t
				}
			}
		}
	}

	var nameFK, nameSem string
	var nameStand time.Time
	if m := filenameRE.FindStringSubmatch(filename); m != nil {
		nameSem, nameFK = m[1], normFK(m[2])
		if t, err := time.ParseInLocation("02-01-06", m[3]+"-"+m[4]+"-"+m[5], time.Local); err == nil {
			nameStand = t
		}
	}

	pick := func(what, cell, name string) string {
		switch {
		case cell != "" && name != "" && cell != name:
			list.Warnings = append(list.Warnings, fmt.Sprintf(
				"%s laut Tabelle %s, laut Dateiname %s — verwendet wird der Wert aus der Tabelle", what, cell, name))
			return cell
		case cell != "":
			return cell
		default:
			return name
		}
	}
	list.FK = pick("Fakultät", cellFK, nameFK)
	list.SemesterCode = pick("Semester", cellSem, nameSem)
	switch {
	case !cellStand.IsZero():
		list.Stand = cellStand
		if !nameStand.IsZero() && nameStand.Format("2006-01-02") != cellStand.Format("2006-01-02") {
			list.Warnings = append(list.Warnings, fmt.Sprintf(
				"Stand laut Tabelle %s, laut Dateiname %s — verwendet wird der Wert aus der Tabelle",
				cellStand.Format("02.01.2006"), nameStand.Format("02.01.2006")))
		}
	case !nameStand.IsZero():
		list.Stand = nameStand
	default:
		list.Warnings = append(list.Warnings, "Stand der Liste weder in der Tabelle noch im Dateinamen gefunden")
	}
	if list.FK == "" {
		list.Warnings = append(list.Warnings, "Fakultät weder in der Tabelle noch im Dateinamen gefunden")
	}
	if list.SemesterCode == "" {
		list.Warnings = append(list.Warnings, "Semester weder in der Tabelle noch im Dateinamen gefunden")
	} else if id, err := SemesterID(list.SemesterCode); err != nil {
		list.Warnings = append(list.Warnings, err.Error())
	} else {
		list.SemesterID = id
	}

	cell := func(r []string, col string) string {
		j := cols[col]
		if j >= len(r) {
			return ""
		}
		return strings.TrimSpace(r[j])
	}

	seen := map[string]int{}
	for i, r := range rows[headerIdx+1:] {
		line := headerIdx + 2 + i
		row := &Row{
			Line:          line,
			FK:            normFK(cell(r, "fk")),
			Stg:           cell(r, "stg"),
			Mtknr:         cell(r, "mtknr"),
			Vorname:       cell(r, "vname"),
			Name:          cell(r, "name"),
			BeginnStudium: cell(r, "beginn_studium"),
			Compensation:  CleanCompensation(cell(r, "nachteilsausgleich")),
			Validity:      cell(r, "semester"),
			Regs:          []Reg{},
			Problems:      []string{},
		}
		if row.Mtknr == "" && row.Compensation == "" && row.Stg == "" {
			continue // empty or trailing row
		}
		if row.Mtknr == "" {
			row.Problems = append(row.Problems, "Matrikelnummer fehlt")
		} else if prev, ok := seen[row.Mtknr]; ok {
			row.Problems = append(row.Problems, fmt.Sprintf("Matrikelnummer kommt schon in Zeile %d vor", prev))
		} else {
			seen[row.Mtknr] = line
		}
		if row.Name == "" {
			row.Problems = append(row.Problems, "Name fehlt")
		}
		if row.Stg == "" {
			row.Problems = append(row.Problems, "Studiengang fehlt")
		}
		if row.Compensation == "" {
			row.Problems = append(row.Problems, "Nachteilsausgleich fehlt")
		} else {
			row.Parsed = ParseCompensation(row.Compensation)
		}
		if until, err := ParseValidity(row.Validity); err != nil {
			row.Problems = append(row.Problems, err.Error())
		} else {
			row.Until = until
		}
		regs, bad := ParseRegs(cell(r, "anmeldungen_aktsemester"))
		row.Regs = regs
		if len(bad) > 0 {
			row.Problems = append(row.Problems, fmt.Sprintf("Anmeldungen nicht lesbar: %s", strings.Join(bad, ", ")))
		}
		list.Rows = append(list.Rows, row)
	}
	return list, nil
}

// ParseRegs splits "IF-B:123 | AW-:456" into registrations and returns the parts
// it could not read.
func ParseRegs(raw string) ([]Reg, []string) {
	regs := []Reg{}
	var bad []string
	for _, part := range strings.Split(raw, "|") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		stg, code, ok := strings.Cut(part, ":")
		n, err := strconv.Atoi(strings.TrimSpace(code))
		if !ok || err != nil || strings.TrimSpace(stg) == "" {
			bad = append(bad, part)
			continue
		}
		regs = append(regs, Reg{Stg: strings.TrimSpace(stg), PrimussAncode: n})
	}
	return regs, bad
}
