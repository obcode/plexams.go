package conflictcalc

import (
	"testing"
	"time"

	"github.com/obcode/plexams.go/graph/model"
)

func TestNormPair(t *testing.T) {
	if a, b := NormPair(7, 3); a != 3 || b != 7 {
		t.Errorf("NormPair(7,3) = (%d,%d), want (3,7)", a, b)
	}
	if a, b := NormPair(3, 7); a != 3 || b != 7 {
		t.Errorf("NormPair(3,7) = (%d,%d), want (3,7)", a, b)
	}
	if a, b := NormPair(5, 5); a != 5 || b != 5 {
		t.Errorf("NormPair(5,5) = (%d,%d), want (5,5)", a, b)
	}
}

func TestProximityRank(t *testing.T) {
	tests := []struct {
		label string
		want  int
	}{
		{Overlap, 4},
		{TooClose, 3},
		{SameDay, 2},
		{NextDay, 1},
		{"", 0},
		{"UNKNOWN", 0},
	}
	for _, tt := range tests {
		if got := ProximityRank(tt.label); got != tt.want {
			t.Errorf("ProximityRank(%q) = %d, want %d", tt.label, got, tt.want)
		}
	}
	// ranking must be strictly worst→mildest so worse/better comparisons hold
	ordered := []string{Overlap, TooClose, SameDay, NextDay, ""}
	for i := 1; i < len(ordered); i++ {
		if ProximityRank(ordered[i-1]) <= ProximityRank(ordered[i]) {
			t.Errorf("proximity ranks not strictly ordered at %q vs %q", ordered[i-1], ordered[i])
		}
	}
}

func TestTimeProximity(t *testing.T) {
	const (
		examGap     = 30  // required break (minutes)
		notTooClose = 120 // same-day start-to-start warning threshold (minutes)
	)
	// day 1 = Mon 12 Jan 2026
	at := func(day, hour, min int) time.Time {
		return time.Date(2026, 1, 11+day, hour, min, 0, 0, time.Local)
	}
	end := func(t time.Time, durMin int) time.Time { return t.Add(time.Duration(durMin) * time.Minute) }

	tests := []struct {
		name      string
		startA    time.Time
		durA      int
		startB    time.Time
		durB      int
		wantRank  int
		wantLabel string
	}{
		{"identical start → overlap", at(1, 8, 30), 90, at(1, 8, 30), 90, 4, Overlap},
		{"intervals overlap → overlap", at(1, 8, 30), 90, at(1, 9, 30), 90, 4, Overlap},
		{"break shorter than gap → overlap", at(1, 8, 30), 90, at(1, 10, 15), 90, 4, Overlap},          // 15min break < 30
		{"too close (enough break, <120 start gap)", at(1, 8, 30), 60, at(1, 10, 15), 90, 3, TooClose}, // break 45, start gap 105
		{"same day far enough (120 grid)", at(1, 8, 30), 90, at(1, 10, 30), 90, 2, SameDay},            // break 30, start gap 120
		{"next day", at(1, 8, 30), 90, at(2, 8, 30), 90, 1, NextDay},
		{"next day reversed", at(2, 8, 30), 90, at(1, 8, 30), 90, 1, NextDay},
		{"far apart", at(1, 8, 30), 90, at(5, 8, 30), 90, 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rank, label := TimeProximity(tt.startA, end(tt.startA, tt.durA), tt.startB, end(tt.startB, tt.durB), examGap, notTooClose)
			if rank != tt.wantRank || label != tt.wantLabel {
				t.Errorf("TimeProximity = (%d, %q), want (%d, %q)", rank, label, tt.wantRank, tt.wantLabel)
			}
		})
	}
}

func conflict(a1, a2 int, prox string) *model.ExamScheduleConflict {
	return &model.ExamScheduleConflict{Ancode1: a1, Ancode2: a2, Proximity: prox}
}

func TestDiffAgainstSaved(t *testing.T) {
	saved := []*model.ExamScheduleConflict{
		conflict(1, 2, TooClose), // will stay TOO_CLOSE -> unchanged
		conflict(1, 3, SameDay),  // will become TOO_CLOSE -> worse
		conflict(1, 4, Overlap),  // will become SameDay -> better
		conflict(1, 5, TooClose), // gone in generated -> resolved
	}
	generated := []*model.ExamScheduleConflict{
		conflict(1, 2, TooClose),
		conflict(1, 3, TooClose),
		conflict(1, 4, SameDay),
		conflict(1, 6, Overlap), // not in saved -> new
	}

	resolved := DiffAgainstSaved(generated, saved)

	wantGen := map[[2]int]string{
		{1, 2}: "unchanged",
		{1, 3}: "worse",
		{1, 4}: "better",
		{1, 6}: "new",
	}
	for _, c := range generated {
		key := [2]int{c.Ancode1, c.Ancode2}
		if c.DiffStatus != wantGen[key] {
			t.Errorf("generated %v DiffStatus = %q, want %q", key, c.DiffStatus, wantGen[key])
		}
	}

	if len(resolved) != 1 {
		t.Fatalf("resolved count = %d, want 1", len(resolved))
	}
	if resolved[0].Ancode1 != 1 || resolved[0].Ancode2 != 5 {
		t.Errorf("resolved pair = (%d,%d), want (1,5)", resolved[0].Ancode1, resolved[0].Ancode2)
	}
	if resolved[0].DiffStatus != "resolved" {
		t.Errorf("resolved DiffStatus = %q, want %q", resolved[0].DiffStatus, "resolved")
	}
}

func TestDiffAgainstSavedEmpty(t *testing.T) {
	// no saved plan: everything is new, nothing resolved
	generated := []*model.ExamScheduleConflict{conflict(1, 2, Overlap)}
	resolved := DiffAgainstSaved(generated, nil)
	if len(resolved) != 0 {
		t.Errorf("resolved = %d, want 0", len(resolved))
	}
	if generated[0].DiffStatus != "new" {
		t.Errorf("DiffStatus = %q, want new", generated[0].DiffStatus)
	}
}
