package plexams

import "testing"

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

func TestConflictSeverityRankOrdersMostSevereFirst(t *testing.T) {
	if conflictSeverityRank(conflictSameSlot) >= conflictSeverityRank(conflictAdjacent) ||
		conflictSeverityRank(conflictAdjacent) >= conflictSeverityRank(conflictSameDay) {
		t.Errorf("severity order wrong: sameSlot=%d adjacent=%d sameDay=%d",
			conflictSeverityRank(conflictSameSlot), conflictSeverityRank(conflictAdjacent), conflictSeverityRank(conflictSameDay))
	}
}
