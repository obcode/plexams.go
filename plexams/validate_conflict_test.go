package plexams

import (
	"testing"

	"github.com/obcode/plexams.go/graph/model"
)

func TestAcceptedKeyNormalizesAncodeOrder(t *testing.T) {
	a := acceptedKey("mtk1", 113, 112)
	b := acceptedKey("mtk1", 112, 113)
	if a != b {
		t.Errorf("acceptedKey must be order-independent: %+v != %+v", a, b)
	}
	if a.ancode1 != 112 || a.ancode2 != 113 {
		t.Errorf("acceptedKey must sort ancodes ascending, got %+v", a)
	}
	if c := acceptedKey("mtk2", 112, 113); c == a {
		t.Errorf("different mtknr must give a different key: %+v == %+v", c, a)
	}
}

func TestConflictLevelRule(t *testing.T) {
	// everything the user allows is only info: a pair-level allowance (sameSlot
	// constraint / canShareSlot) or all affected students accepted (real == 0).
	cases := []struct {
		name    string
		problem string
		real    int
		allowed bool
		want    model.ValidationLevel
	}{
		{"overlap, real, not allowed", conflictOverlap, 2, false, model.ValidationLevelError},
		{"overlap, pair allowed", conflictOverlap, 2, true, model.ValidationLevelInfo},
		{"overlap, all accepted", conflictOverlap, 0, false, model.ValidationLevelInfo},
		{"too close, real", conflictTooClose, 1, false, model.ValidationLevelWarning},
		{"too close, allowed", conflictTooClose, 1, true, model.ValidationLevelInfo},
		{"same day, real", conflictSameDay, 3, false, model.ValidationLevelInfo},
	}
	for _, c := range cases {
		if got := conflictLevel(c.problem, c.real, c.allowed); got != c.want {
			t.Errorf("%s: conflictLevel(%q, %d, %v) = %v, want %v", c.name, c.problem, c.real, c.allowed, got, c.want)
		}
	}
}

func TestConflictSeverityRankOrdersMostSevereFirst(t *testing.T) {
	if conflictSeverityRank(conflictOverlap) >= conflictSeverityRank(conflictTooClose) ||
		conflictSeverityRank(conflictTooClose) >= conflictSeverityRank(conflictSameDay) {
		t.Errorf("severity order wrong: overlap=%d tooClose=%d sameDay=%d",
			conflictSeverityRank(conflictOverlap), conflictSeverityRank(conflictTooClose), conflictSeverityRank(conflictSameDay))
	}
}
