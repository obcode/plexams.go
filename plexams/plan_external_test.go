package plexams

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
	"github.com/obcode/plexams.go/plexams/apierr"
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

// A joint exam owned by another faculty lives in the same table as the ZPA
// exams. Setting its external time must not be mistaken for a ZPA exam and
// refused for lacking "nicht von mir geplant".
func TestExternalTimeForJointExam(t *testing.T) {
	pg := pgtest.NewDBWithSemester(t)
	p := &Plexams{dbClient: pg}
	ctx := context.Background()
	if err := pg.AddExternalExam(ctx, &model.ZPAExam{
		AnCode: 90013, Module: "Medizinische Grundlagen", MainExamer: "FK11",
		ExamType: "sp", ExamTypeFull: "schriftliche Prüfung", Groups: []string{},
	}); err != nil {
		t.Fatal(err)
	}

	at := time.Date(2027, 2, 3, 10, 30, 0, 0, time.Local)
	if ok, err := p.AddExamToSlottime(ctx, 90013, at); err != nil || !ok {
		t.Fatalf("want the external time stored, got ok=%v err=%v", ok, err)
	}
	entry, err := pg.PlanEntry(ctx, 90013)
	if err != nil {
		t.Fatal(err)
	}
	if entry == nil || entry.Starttime == nil || !entry.Starttime.Equal(at) || !entry.External {
		t.Errorf("want an external plan entry at %v, got %+v", at, entry)
	}
}

func TestExternalTimeForUnknownAncode(t *testing.T) {
	pg := pgtest.NewDBWithSemester(t)
	p := &Plexams{dbClient: pg}
	_, err := p.AddExamToSlottime(context.Background(), 4711, time.Date(2027, 2, 3, 10, 30, 0, 0, time.Local))
	if apierr.Code(err) != apierr.CodeNotFound {
		t.Errorf("want not found, got %v", err)
	}
}

// GetZpaExamByAncode is ZPA only; ExamByAncode finds an exam of either source.
func TestExamByAncodeFindsBothSources(t *testing.T) {
	pg := pgtest.NewDBWithSemester(t)
	p := &Plexams{dbClient: pg}
	ctx := context.Background()
	if err := pg.CacheZPAExams([]*model.ZPAExam{{
		AnCode: 100, Module: "Analysis", MainExamer: "Test", MainExamerID: 1,
		ExamType: "sp", ExamTypeFull: "schriftliche Prüfung", Groups: []string{},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := pg.AddExternalExam(ctx, &model.ZPAExam{
		AnCode: 90013, Module: "Medizinische Grundlagen", MainExamer: "FK11",
		ExamType: "sp", Groups: []string{},
	}); err != nil {
		t.Fatal(err)
	}

	for ancode, module := range map[int]string{100: "Analysis", 90013: "Medizinische Grundlagen"} {
		if exam, err := p.ExamByAncode(ctx, ancode); err != nil || exam == nil || exam.Module != module {
			t.Errorf("ExamByAncode(%d): got %+v, %v", ancode, exam, err)
		}
	}
	if exam, err := p.GetZpaExamByAncode(ctx, 90013); err == nil {
		t.Errorf("GetZpaExamByAncode(90013) must not find the external exam, got %+v", exam)
	}
	if _, err := p.ExamByAncode(ctx, 4711); err == nil {
		t.Error("ExamByAncode(4711): want an error for an unknown ancode")
	}
}
