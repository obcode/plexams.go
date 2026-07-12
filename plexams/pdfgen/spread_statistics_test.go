package pdfgen

import (
	"testing"

	"github.com/obcode/plexams.go/graph/model"
)

// TestSpreadStatisticsRenders exercises the full maroto layout (all TableLists,
// section rows, footer) end-to-end and asserts it renders to non-empty PDF bytes —
// catching column/grid mismatches and nil handling without needing a database.
func TestSpreadStatisticsRenders(t *testing.T) {
	stat := &model.ExamSpreadStatistics{
		StudentCount:               420,
		MultiExamStudentCount:      380,
		TotalPlannedExams:          1400,
		StudentsWithUnplannedExams: 7,
		AvgExamsPerStudent:         3.33,
		MaxExamsPerStudent:         7,
		FreeDayShare:               72.4,
		SameDayShare:               11.1,
		AdjacentDayShare:           14.2,
		ConflictShare:              0,
		ThreeExamsOneDayCount:      3,
		AvgMinFreeDays:             1.4,
		MedianMinFreeDays:          1,
		AvgProximityCost:           9.6,
		ExamGapMinutes:             30,
		NotTooCloseMinutes:         90,
		StudentBuckets: []*model.SpreadBucket{
			{Key: "OVERLAP", Label: "Überschneidung (Konflikt)", Count: 0, Share: 0},
			{Key: "SAME_DAY", Label: "Zwei Prüfungen am selben Tag", Count: 42, Share: 11.1},
			{Key: "ADJACENT", Label: "Aufeinanderfolgende Tage (kein freier Tag)", Count: 54, Share: 14.2},
			{Key: "ONE_FREE", Label: "1 freier Tag dazwischen", Count: 120, Share: 31.6},
			{Key: "TWO_FREE", Label: "2 freie Tage dazwischen", Count: 100, Share: 26.3},
			{Key: "THREE_PLUS_FREE", Label: "3+ freie Tage dazwischen", Count: 64, Share: 16.8},
		},
		ExamCountBuckets: []*model.CountBucket{
			{ExamCount: 1, Label: "1 Prüfung", Students: 40, Share: 9.5},
			{ExamCount: 3, Label: "3 Prüfungen", Students: 180, Share: 42.9},
			{ExamCount: 6, Label: "6+ Prüfungen", Students: 20, Share: 4.8},
		},
		ByProgram: []*model.ProgramSpread{
			{Program: "IF", StudentCount: 200, MultiExamStudentCount: 190, AvgExamsPerStudent: 3.4, FreeDayShare: 75, SameDayShare: 9, AvgMinFreeDays: 1.5},
			{Program: "DC", StudentCount: 90, MultiExamStudentCount: 80, AvgExamsPerStudent: 3.1, FreeDayShare: 60, SameDayShare: 20, AvgMinFreeDays: 0.9},
		},
	}

	m := SpreadStatistics("Sommersemester 2026", stat)
	buf, err := m.Output()
	if err != nil {
		t.Fatalf("Output() error: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("rendered PDF is empty")
	}
}

// TestSpreadStatisticsEmpty renders the zero-data case (no plan yet) without panicking.
func TestSpreadStatisticsEmpty(t *testing.T) {
	stat := &model.ExamSpreadStatistics{
		StudentBuckets:   buildEmptyBuckets(),
		ExamCountBuckets: []*model.CountBucket{},
		ByProgram:        []*model.ProgramSpread{},
	}
	m := SpreadStatistics("Wintersemester 2026/27", stat)
	if _, err := m.Output(); err != nil {
		t.Fatalf("Output() on empty stat error: %v", err)
	}
}

func buildEmptyBuckets() []*model.SpreadBucket {
	keys := []string{"OVERLAP", "SAME_DAY", "ADJACENT", "ONE_FREE", "TWO_FREE", "THREE_PLUS_FREE"}
	out := make([]*model.SpreadBucket, 0, len(keys))
	for _, k := range keys {
		out = append(out, &model.SpreadBucket{Key: k, Label: k, Count: 0, Share: 0})
	}
	return out
}
