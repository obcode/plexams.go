package plexams

import (
	"testing"

	"github.com/obcode/plexams.go/graph/model"
)

func TestSemesterOrdinal(t *testing.T) {
	cases := []struct {
		label   string
		wantOK  bool
		wantOrd int
	}{
		{"2026 SS", true, 2026*2 + 0},
		{"2026-SS", true, 2026*2 + 0},
		{"2026ss", true, 2026*2 + 0},
		{"2025 WS", true, 2025*2 + 1},
		{"2025-WS", true, 2025*2 + 1},
		{" 2026-SS ", true, 2026*2 + 0},
		{"2026 SS-Test", false, 0}, // clone name, no SS/WS suffix
		{"garbage", false, 0},
		{"", false, 0},
	}
	for _, c := range cases {
		ord, ok := semesterOrdinal(c.label)
		if ok != c.wantOK || (ok && ord != c.wantOrd) {
			t.Errorf("semesterOrdinal(%q) = (%d, %v), want (%d, %v)", c.label, ord, ok, c.wantOrd, c.wantOK)
		}
	}

	// chronological order: WS starts the academic year, so 2025 WS < 2026 SS < 2026 WS.
	ws25, _ := semesterOrdinal("2025 WS")
	ss26, _ := semesterOrdinal("2026 SS")
	ws26, _ := semesterOrdinal("2026 WS")
	if ws25 >= ss26 || ss26 >= ws26 {
		t.Errorf("expected 2025WS(%d) < 2026SS(%d) < 2026WS(%d)", ws25, ss26, ws26)
	}
}

func TestPermanentAppliesTo(t *testing.T) {
	s := func(v string) *string { return &v }

	entry := func(from, until *string) *model.PermanentNonInvigilator {
		return &model.PermanentNonInvigilator{TeacherID: 1, ValidFrom: from, ValidUntil: until}
	}
	ord := func(label string) int { o, _ := semesterOrdinal(label); return o }

	cases := []struct {
		name    string
		n       *model.PermanentNonInvigilator
		cur     string
		curOK   bool
		applies bool
	}{
		{"open range always applies", entry(nil, nil), "2026-SS", true, true},
		{"before validFrom → no", entry(s("2026-SS"), nil), "2025-WS", true, false},
		{"at validFrom → yes", entry(s("2026-SS"), nil), "2026-SS", true, true},
		{"after validFrom → yes", entry(s("2026-SS"), nil), "2026-WS", true, true},
		{"after validUntil → no", entry(nil, s("2026-SS")), "2026-WS", true, false},
		{"at validUntil → yes", entry(nil, s("2026-SS")), "2026-SS", true, true},
		{"within closed range → yes", entry(s("2025-WS"), s("2026-WS")), "2026-SS", true, true},
		{"outside closed range → no", entry(s("2025-WS"), s("2026-SS")), "2026-WS", true, false},
		{"unparseable current → keep exclusion", entry(nil, s("2020-SS")), "clone-xyz", false, true},
		{"unparseable bound → ignored (keep)", entry(s("nonsense"), nil), "2026-SS", true, true},
	}
	for _, c := range cases {
		got := permanentAppliesTo(c.n, ord(c.cur), c.curOK)
		if got != c.applies {
			t.Errorf("%s: permanentAppliesTo() = %v, want %v", c.name, got, c.applies)
		}
	}
}
