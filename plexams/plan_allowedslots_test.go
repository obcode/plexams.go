package plexams

import (
	"sort"
	"testing"
	"time"

	"github.com/obcode/plexams.go/graph/model"
)

func TestEffectiveGapMinutes(t *testing.T) {
	// same campus (incl. both empty/default) → ordinary examGap; different campus → the
	// larger cross-campus travel buffer.
	cases := []struct {
		locA, locB string
		want       int
	}{
		{"", "", 30},
		{"Pasing", "Pasing", 30},
		{"", "Pasing", defaultCrossCampusGapMinutes},
		{"Pasing", "", defaultCrossCampusGapMinutes},
		{"Pasing", "Lothstr", defaultCrossCampusGapMinutes},
	}
	for _, c := range cases {
		if got := effectiveGapMinutes(30, defaultCrossCampusGapMinutes, c.locA, c.locB); got != c.want {
			t.Errorf("effectiveGapMinutes(30, %q, %q) = %d, want %d", c.locA, c.locB, got, c.want)
		}
	}
	// a same-campus examGap already larger than the cross-campus buffer is not shrunk.
	if got := effectiveGapMinutes(200, defaultCrossCampusGapMinutes, "", "Pasing"); got != 200 {
		t.Errorf("effectiveGapMinutes must never shrink the gap: got %d, want 200", got)
	}
}

func TestCrossCampusGapMinutesConfigurable(t *testing.T) {
	p := &Plexams{semesterConfig: &model.SemesterConfig{}}
	if got := p.crossCampusGapMinutes(); got != defaultCrossCampusGapMinutes {
		t.Errorf("unset → default: got %d, want %d", got, defaultCrossCampusGapMinutes)
	}
	p.semesterConfig.CrossCampusGapMinutes = 60
	if got := p.crossCampusGapMinutes(); got != 60 {
		t.Errorf("configured value must win: got %d, want 60", got)
	}
	// and it must feed the effective gap for a cross-campus pair
	if got := effectiveGapMinutes(30, p.crossCampusGapMinutes(), "", "Pasing"); got != 60 {
		t.Errorf("effective gap with configured cross-campus buffer: got %d, want 60", got)
	}
}

// slotsOnDay builds grid slots at the given HH:MM on 2026-07-06.
func slotsOnDay(hhmm ...[2]int) []*model.Slot {
	slots := make([]*model.Slot, 0, len(hhmm))
	for _, hm := range hhmm {
		st := time.Date(2026, 7, 6, hm[0], hm[1], 0, 0, time.Local)
		slots = append(slots, &model.Slot{Starttime: st})
	}
	return slots
}

func allowedHHMM(slots []*model.Slot) []string {
	out := make([]string, 0, len(slots))
	for _, s := range slots {
		out = append(out, s.Starttime.Format("15:04"))
	}
	sort.Strings(out)
	return out
}

func newPlexamsWithSlots(slots []*model.Slot, examGap int) *Plexams {
	return &Plexams{semesterConfig: &model.SemesterConfig{
		Slots:          slots,
		ForbiddenSlots: []*model.Slot{},
		ExamGapMinutes: examGap,
	}}
}

// The core regression: a conflicting exam fixed at an OFF-GRID time (11:00) must block the
// grid slot whose exam window overlaps it (10:30), while non-overlapping slots stay allowed.
func TestAllowedSlotsForExcludesOffGridOverlap(t *testing.T) {
	slots := slotsOnDay([2]int{8, 30}, [2]int{10, 30}, [2]int{12, 30})
	p := newPlexamsWithSlots(slots, 30)

	exam := &model.AssembledExam{
		Ancode:      424,
		MaxDuration: 90,
		Conflicts:   []*model.ZPAConflict{{Ancode: 338}},
	}
	placed := map[int]placedExamInfo{
		338: {start: time.Date(2026, 7, 6, 11, 0, 0, 0, time.Local), duration: 60, location: "", fixed: true},
	}

	got := allowedHHMM(p.allowedSlotsFor(exam, placed, p.examGapMinutes()))
	want := []string{"08:30", "12:30"} // 10:30 (10:30–12:00) overlaps 11:00–12:00 → excluded
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("off-grid overlap: allowed = %v, want %v", got, want)
	}
}

// Cross-campus: the travel buffer widens the excluded window well beyond the overlapping
// slot — an exam on the main campus may not sit within the travel buffer of a Pasing exam.
func TestAllowedSlotsForCrossCampusWiderBuffer(t *testing.T) {
	slots := slotsOnDay([2]int{8, 30}, [2]int{10, 30}, [2]int{12, 30}, [2]int{14, 30})
	p := newPlexamsWithSlots(slots, 30)

	exam := &model.AssembledExam{
		Ancode:      424,
		MaxDuration: 90,
		Conflicts:   []*model.ZPAConflict{{Ancode: 900}},
	}
	// Pasing conflict 10:30–12:00. cross-campus buffer = 120 min end-to-start.
	placed := map[int]placedExamInfo{
		900: {start: time.Date(2026, 7, 6, 10, 30, 0, 0, time.Local), duration: 90, location: "Campus Pasing", fixed: true},
	}

	got := allowedHHMM(p.allowedSlotsFor(exam, placed, p.examGapMinutes()))
	// 08:30 (ends 10:00, gap 30 < 120), 10:30 (overlap), 12:30 (conflict ends 12:00, gap 30
	// < 120) all excluded; only 14:30 (gap 150 > 120) survives.
	want := []string{"14:30"}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("cross-campus buffer: allowed = %v, want %v", got, want)
	}
}

// A conflicting exam that is not placed anywhere (no absolute time) excludes nothing.
func TestOverlapSlotsSkipsUnplacedConflict(t *testing.T) {
	slots := slotsOnDay([2]int{8, 30}, [2]int{10, 30})
	p := newPlexamsWithSlots(slots, 30)
	exam := &model.AssembledExam{Ancode: 424, MaxDuration: 90, Conflicts: []*model.ZPAConflict{{Ancode: 338}}}
	if excl := p.overlapSlots(exam, map[int]placedExamInfo{}, 30); excl.Cardinality() != 0 {
		t.Errorf("unplaced conflict must exclude no slot, got %d", excl.Cardinality())
	}
}
