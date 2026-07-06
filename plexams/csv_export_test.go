package plexams

import (
	"testing"
	"time"

	"github.com/obcode/plexams.go/graph/model"
)

func TestEncodeDecodeCSVRoundTrip(t *testing.T) {
	header := []string{"ancode", "toPlan"}
	rows := [][]string{{"112", "true"}, {"130", "false"}}
	data, err := encodeCSV(header, rows)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	// must carry a UTF-8 BOM for Excel
	if len(data) < 3 || data[0] != 0xEF || data[1] != 0xBB || data[2] != 0xBF {
		t.Errorf("missing UTF-8 BOM")
	}
	got, err := decodeCSV(data, header)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 || got[0][0] != "112" || got[1][1] != "false" {
		t.Errorf("round-trip mismatch: %v", got)
	}
}

// TestDecodeCSVRejectsWrongHeader is the guard that prevents applying a file to the
// wrong dataset (the CSV analogue of the earlier silent-wipe bug).
func TestDecodeCSVRejectsWrongHeader(t *testing.T) {
	data, _ := encodeCSV([]string{"ancode", "duration"}, [][]string{{"1", "90"}})
	if _, err := decodeCSV(data, []string{"ancode", "toPlan"}); err == nil {
		t.Errorf("expected header-mismatch error, got nil")
	}
}

func TestListHelpers(t *testing.T) {
	if ints2s([]int{1, 2, 3}) != "1;2;3" {
		t.Errorf("ints2s: %q", ints2s([]int{1, 2, 3}))
	}
	got, err := s2ints("1; 2 ;3")
	if err != nil || len(got) != 3 || got[2] != 3 {
		t.Errorf("s2ints: %v %v", got, err)
	}
	if s2ints2empty := func() int { x, _ := s2ints(""); return len(x) }(); s2ints2empty != 0 {
		t.Errorf("s2ints empty should be nil/0")
	}
	if !s2b("Ja") || !s2b("x") || s2b("nein") {
		t.Errorf("s2b tolerant parse failed")
	}
}

func TestDatesRoundTrip(t *testing.T) {
	d1 := time.Date(2026, 2, 3, 0, 0, 0, 0, time.Local)
	d2 := time.Date(2026, 3, 4, 0, 0, 0, 0, time.Local)
	s := dates2s([]*time.Time{&d1, &d2})
	if s != "03.02.2026;04.03.2026" {
		t.Fatalf("dates2s: %q", s)
	}
	back, err := s2dates(s)
	if err != nil || len(back) != 2 || !back[0].Equal(d1) || !back[1].Equal(d2) {
		t.Errorf("s2dates: %v %v", back, err)
	}

	dt := time.Date(2026, 2, 3, 14, 30, 0, 0, time.Local)
	if dateTimePtr2s(&dt) != "03.02.2026 14:30" {
		t.Errorf("dateTimePtr2s: %q", dateTimePtr2s(&dt))
	}
	got, err := s2dateTimePtr("03.02.2026 14:30")
	if err != nil || got == nil || !got.Equal(dt) {
		t.Errorf("s2dateTimePtr: %v %v", got, err)
	}
}

func TestPrimussAncodesRoundTrip(t *testing.T) {
	in := []model.ZPAPrimussAncodes{{Program: "IF", Ancode: 123}, {Program: "DE", Ancode: 200}}
	s := primussAncodes2s(in)
	if s != "IF:123;DE:200" {
		t.Fatalf("primussAncodes2s: %q", s)
	}
	back, err := s2primussAncodes(s)
	if err != nil || len(back) != 2 || back[1].Program != "DE" || back[1].Ancode != 200 {
		t.Errorf("s2primussAncodes: %v %v", back, err)
	}
	if _, err := s2primussAncodes("bad-entry"); err == nil {
		t.Errorf("expected error for malformed primussAncode")
	}
}
