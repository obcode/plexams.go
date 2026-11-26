package plexams

import (
	"context"
	"github.com/obcode/plexams.go/plexams/apierr"
	"strings"
	"testing"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
)

// A ZPA exam without any constraints row is one nobody marked as planned
// elsewhere: setting an external time must be refused, not panic on the missing
// row.
func TestExternalTimeForExamWithoutConstraints(t *testing.T) {
	pg := pgtest.NewDBWithSemester(t)
	p := &Plexams{dbClient: pg}
	if err := pg.CacheZPAExams([]*model.ZPAExam{{
		AnCode: 100, Module: "Analysis", MainExamer: "Test", MainExamerID: 1,
		ExamType: "sp", ExamTypeFull: "schriftliche Prüfung", Groups: []string{},
	}}); err != nil {
		t.Fatal(err)
	}

	_, err := p.AddExamToSlottime(context.Background(), 100, time.Date(2027, 2, 3, 10, 30, 0, 0, time.Local))
	if apierr.Code(err) != apierr.CodeInvalidInput || !strings.Contains(err.Error(), "nicht von mir geplant") {
		t.Errorf("want the not-planned-by-me refusal, got %v", err)
	}
}
