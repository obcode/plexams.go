package ntalist

import (
	"bytes"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func TestSemesterID(t *testing.T) {
	for code, want := range map[string]string{"226": "2026-WS", "127": "2027-SS", "199": "2099-SS"} {
		got, err := SemesterID(code)
		if err != nil || got != want {
			t.Errorf("SemesterID(%q) = %q, %v; want %q", code, got, err, want)
		}
	}
	for _, bad := range []string{"", "26", "326", "2266", "abc"} {
		if _, err := SemesterID(bad); err == nil {
			t.Errorf("SemesterID(%q) should fail", bad)
		}
	}
}

func TestParseValidity(t *testing.T) {
	for raw, want := range map[string]string{"*": UntilEndOfStudies, " * ": UntilEndOfStudies,
		"226:127": "2027-SS", "226": "2026-WS", "225:126:226": "2026-WS"} {
		got, err := ParseValidity(raw)
		if err != nil || got != want {
			t.Errorf("ParseValidity(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
	for _, bad := range []string{"", "bis Ende", "226:"} {
		if _, err := ParseValidity(bad); err == nil {
			t.Errorf("ParseValidity(%q) should fail", bad)
		}
	}
}

func TestParseCompensation(t *testing.T) {
	tests := []struct {
		text string
		want Compensation
	}{
		{"10% Schreibzeitverlängerung", Compensation{DeltaDurationPercent: 10}},
		{"30 % Schreibzeitverlängerung", Compensation{DeltaDurationPercent: 30}},
		{"10% Schreibzeitverlängerung, gesonderter Raum", Compensation{DeltaDurationPercent: 10, NeedsRoomAlone: true}},
		{"10% Schreibzeitverlängerung in gesondertem Raum, 20% Zeitverlängerung bei mündlichen Prüfungen",
			Compensation{DeltaDurationPercent: 10, NeedsRoomAlone: true}},
		{"50% Schreibzeitverlängerung mit Assistenz im gesonderten Raum, Computerarbeitsplatz mit Screenreader und eigene Braillezeile",
			Compensation{DeltaDurationPercent: 50, NeedsRoomAlone: true, NeedsHardware: true}},
		{"Einzelraum", Compensation{NeedsRoomAlone: true, Unparsed: true}},
		{"20% Zeitverlängerung bei mündlichen Prüfungen", Compensation{Unparsed: true}},
	}
	for _, tt := range tests {
		if got := ParseCompensation(tt.text); got != tt.want {
			t.Errorf("ParseCompensation(%q) = %+v; want %+v", tt.text, got, tt.want)
		}
	}
}

func TestParseRegs(t *testing.T) {
	regs, bad := ParseRegs("IF-B:101 | AW-:202 |  | XX | IF-B:abc")
	if len(regs) != 2 || regs[0] != (Reg{"IF-B", 101}) || regs[1] != (Reg{"AW-", 202}) {
		t.Errorf("regs = %+v", regs)
	}
	if len(bad) != 2 {
		t.Errorf("bad = %v", bad)
	}
}

// fixture builds a list in the layout of the PRIMUSS export, with invented data.
func fixture(t *testing.T, header [][]string, rows [][]string) []byte {
	t.Helper()
	f := excelize.NewFile()
	all := append(append([][]string{}, header...), []string{"fk", "stg", "mtknr", "vname", "name",
		"beginn_studium", "nachteilsausgleich", "semester", "anmeldungen_aktsemester"})
	all = append(all, rows...)
	for i, r := range all {
		for j, v := range r {
			if v == "" {
				continue
			}
			cell, _ := excelize.CoordinatesToCellName(j+1, i+1)
			if err := f.SetCellStr("Sheet1", cell, v); err != nil {
				t.Fatal(err)
			}
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

var headerBlock = [][]string{
	{"Hochschule München", "", "", "", "", "", "", "", "27.11.2026 10:05:29"},
	{"Nachteilsausgleich (1234)", "", "", "", "", "", "", "", "primas"},
	{"STG-Abs: *"},
	{"FK 07"},
	{"für Semester 226"},
}

func TestParse(t *testing.T) {
	data := fixture(t, headerBlock, [][]string{
		{"7", "IF-B", "00000001", "Erika", "Muster", "225", "10% Schreibzeitverlängerung", "*", "IF-B:101 | IF-B:102"},
		{"7", "DC-B", "00000002", "Max", "Beispiel", "224", "25% Schreibzeitverlängerung, gesonderter Raum", "226:127", "DC-B:201 | AW-:301"},
		{"7", "IF-B", "00000001", "Erika", "Muster", "225", "10% Schreibzeitverlängerung", "*", ""},
		{"7", "WD-B", "00000003", "Anna", "Test", "226", "Sonderregelung", "irgendwann", "WD-B:1x"},
		{},
	})
	list, err := Parse("226_07_Liste_NTA_Stand_27-11-26.xlsx", data)
	if err != nil {
		t.Fatal(err)
	}
	if list.FK != "07" || list.SemesterCode != "226" || list.SemesterID != "2026-WS" {
		t.Errorf("header = %s %s %s", list.FK, list.SemesterCode, list.SemesterID)
	}
	if want := time.Date(2026, 11, 27, 10, 5, 29, 0, time.Local); !list.Stand.Equal(want) {
		t.Errorf("stand = %v", list.Stand)
	}
	if len(list.Warnings) != 0 {
		t.Errorf("warnings = %v", list.Warnings)
	}
	if len(list.Rows) != 4 {
		t.Fatalf("rows = %d", len(list.Rows))
	}
	r := list.Rows[0]
	if r.Line != 7 || r.FK != "07" || r.Mtknr != "00000001" || r.Until != UntilEndOfStudies ||
		r.Parsed.DeltaDurationPercent != 10 || len(r.Regs) != 2 || len(r.Problems) != 0 {
		t.Errorf("row 0 = %+v", r)
	}
	r = list.Rows[1]
	if r.Until != "2027-SS" || !r.Parsed.NeedsRoomAlone || r.Parsed.DeltaDurationPercent != 25 || len(r.Problems) != 0 {
		t.Errorf("row 1 = %+v", r)
	}
	if len(list.Rows[2].Problems) != 1 {
		t.Errorf("duplicate mtknr not reported: %v", list.Rows[2].Problems)
	}
	r = list.Rows[3]
	if !r.Parsed.Unparsed || len(r.Problems) != 2 {
		t.Errorf("row 3 problems = %v, parsed = %+v", r.Problems, r.Parsed)
	}
}

func TestParseHeaderFromFilenameAndConflicts(t *testing.T) {
	data := fixture(t, [][]string{{"FK 21"}}, [][]string{
		{"21", "DE-B", "00000009", "Kim", "Probe", "226", "10% Schreibzeitverlängerung", "*", ""},
	})
	list, err := Parse("abc-226_07_Liste_NTA_Stand_01-12-26.xlsx", data)
	if err != nil {
		t.Fatal(err)
	}
	if list.FK != "21" || list.SemesterID != "2026-WS" {
		t.Errorf("FK %s semester %s", list.FK, list.SemesterID)
	}
	if !list.Stand.Equal(time.Date(2026, 12, 1, 0, 0, 0, 0, time.Local)) {
		t.Errorf("stand = %v", list.Stand)
	}
	if len(list.Warnings) != 1 {
		t.Errorf("want one warning (FK conflict), got %v", list.Warnings)
	}
}

func TestParseRejects(t *testing.T) {
	if _, err := Parse("liste.csv", nil); err == nil {
		t.Error("csv accepted")
	}
	if _, err := Parse("liste.xlsx", []byte("nope")); err == nil {
		t.Error("garbage accepted")
	}
	f := excelize.NewFile()
	_ = f.SetCellStr("Sheet1", "A1", "irgendwas")
	var buf bytes.Buffer
	_ = f.Write(&buf)
	if _, err := Parse("liste.xlsx", buf.Bytes()); err == nil {
		t.Error("sheet without header accepted")
	}
}
