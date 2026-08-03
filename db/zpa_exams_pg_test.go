package db_test

import (
	"testing"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
)

func testZPAExam(ancode int, module string, groups ...string) *model.ZPAExam {
	if groups == nil {
		groups = []string{}
	}
	return &model.ZPAExam{
		ZpaID:          1000 + ancode,
		AnCode:         ancode,
		Module:         module,
		MainExamer:     "Braun O.",
		MainExamerID:   42,
		ExamType:       "schriftlich",
		ExamTypeFull:   "schriftliche Prüfung",
		Date:           "19.07.2027",
		Starttime:      "14:30",
		Duration:       90,
		IsRepeaterExam: false,
		Groups:         groups,
		PrimussAncodes: []model.ZPAPrimussAncodes{},
	}
}

func TestZPAExamRoundTrip(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")

	want := testZPAExam(100, "Analysis", "IF4B", "WD2B")
	if err := pg.CacheZPAExams([]*model.ZPAExam{want}); err != nil {
		t.Fatalf("CacheZPAExams: %v", err)
	}

	got, err := pg.GetZpaExamByAncode(ctx, 100)
	if err != nil {
		t.Fatalf("GetZpaExamByAncode: %v", err)
	}
	for _, f := range []struct {
		name      string
		want, got string
	}{
		{"Module", want.Module, got.Module},
		{"MainExamer", want.MainExamer, got.MainExamer},
		{"ExamType", want.ExamType, got.ExamType},
		{"ExamTypeFull", want.ExamTypeFull, got.ExamTypeFull},
		{"Date", want.Date, got.Date},
		{"Starttime", want.Starttime, got.Starttime},
	} {
		if f.want != f.got {
			t.Errorf("%s = %q, want %q", f.name, f.got, f.want)
		}
	}
	if got.ZpaID != want.ZpaID || got.MainExamerID != want.MainExamerID || got.Duration != want.Duration {
		t.Errorf("ids/duration = %d/%d/%d", got.ZpaID, got.MainExamerID, got.Duration)
	}
	if len(got.Groups) != 2 || got.Groups[0] != "IF4B" || got.Groups[1] != "WD2B" {
		t.Errorf("Groups = %v, want [IF4B WD2B]", got.Groups)
	}
	// The logical semester comes from the registry rather than a stored column
	// that could drift from it.
	if got.Semester != "2026 WS" {
		t.Errorf("Semester = %q, want %q", got.Semester, "2026 WS")
	}
}

// TestCacheZPAExamsUpsertsAndWithdraws is the difference that matters. The Mongo
// version dropped the collection and re-inserted; here the overlay tables
// reference the exams, so a drop would cascade the planner's constraints away and
// one flaky import would cost a semester of hand-entered work.
func TestCacheZPAExamsUpsertsAndWithdraws(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")

	if err := pg.CacheZPAExams([]*model.ZPAExam{
		testZPAExam(100, "Analysis"),
		testZPAExam(200, "Lineare Algebra"),
		testZPAExam(300, "Programmieren"),
	}); err != nil {
		t.Fatalf("CacheZPAExams: %v", err)
	}
	// The planner does real work on 300.
	exec(t, pg, `insert into exam_constraint (semester_id, ancode, online) values ('2026-WS', 300, true)`)
	exec(t, pg, `insert into exam_duration_override (semester_id, ancode, duration_min) values ('2026-WS', 300, 120)`)

	// Re-import: 100 changed, 400 is new, 300 is gone from ZPA.
	changed := testZPAExam(100, "Analysis (neu)")
	if err := pg.CacheZPAExams([]*model.ZPAExam{
		changed,
		testZPAExam(200, "Lineare Algebra"),
		testZPAExam(400, "Neu"),
	}); err != nil {
		t.Fatalf("CacheZPAExams (second): %v", err)
	}

	exams, err := pg.GetZPAExams(ctx)
	if err != nil {
		t.Fatalf("GetZPAExams: %v", err)
	}
	// The withdrawn one is not delivered any more, exactly as under Mongo ...
	want := []int{100, 200, 400}
	if len(exams) != len(want) {
		t.Fatalf("ancodes = %d exams, want %d", len(exams), len(want))
	}
	for i, w := range want {
		if exams[i].AnCode != w {
			t.Errorf("exams[%d].AnCode = %d, want %d", i, exams[i].AnCode, w)
		}
	}
	if exams[0].Module != "Analysis (neu)" {
		t.Errorf("Module = %q, want the upserted value", exams[0].Module)
	}

	// ... but the row is marked, not gone, and the planner's work survived.
	if n := count(t, pg, `select count(*) from exam where ancode = 300 and withdrawn_at is not null`); n != 1 {
		t.Error("exam 300 should be marked withdrawn, not deleted")
	}
	for _, table := range []string{"exam_constraint", "exam_duration_override"} {
		if n := count(t, pg, `select count(*) from `+table+` where ancode = 300`); n != 1 {
			t.Errorf("%s for the withdrawn exam is gone -- the import destroyed planner input", table)
		}
	}
}

// An exam that comes back clears its withdrawal, so a one-off ZPA hiccup heals
// itself on the next import.
func TestCacheZPAExamsReappearanceClearsTheWithdrawal(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")

	if err := pg.CacheZPAExams([]*model.ZPAExam{testZPAExam(100, "Analysis")}); err != nil {
		t.Fatalf("CacheZPAExams: %v", err)
	}
	if err := pg.CacheZPAExams([]*model.ZPAExam{}); err != nil {
		t.Fatalf("CacheZPAExams (empty): %v", err)
	}
	if n := count(t, pg, `select count(*) from exam where withdrawn_at is not null`); n != 1 {
		t.Fatal("the exam was not withdrawn")
	}

	if err := pg.CacheZPAExams([]*model.ZPAExam{testZPAExam(100, "Analysis")}); err != nil {
		t.Fatalf("CacheZPAExams (third): %v", err)
	}
	exams, err := pg.GetZPAExams(ctx)
	if err != nil {
		t.Fatalf("GetZPAExams: %v", err)
	}
	if len(exams) != 1 {
		t.Errorf("len = %d, want 1 -- the reappearing exam is still withdrawn", len(exams))
	}
}

// A ZPA import delivering nothing is a real case (a fresh semester before the
// exams are entered) and must not error.
func TestCacheZPAExamsWithNoExams(t *testing.T) {
	pg := pgtest.NewDB(t)

	seedPrimussFixtures(t, pg, "IF-B")
	if err := pg.CacheZPAExams([]*model.ZPAExam{}); err != nil {
		t.Fatalf("CacheZPAExams: %v", err)
	}
	exams, err := pg.GetZPAExams(t.Context())
	if err != nil {
		t.Fatalf("GetZPAExams: %v", err)
	}
	if exams == nil {
		t.Fatal("GetZPAExams returned nil, want an empty slice")
	}
}

// The Primuss ancodes ZPA delivers were an array inside the exam document; here
// they are rows, refreshed by the import. -1 means "no Primuss exam yet" and is
// the absence of a link, not a link to ancode -1.
func TestZPAExamPrimussAncodes(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B", "WD-B")
	seedPrimussExam(t, pg, "IF-B", 100, "Analysis")
	seedPrimussExam(t, pg, "WD-B", 100, "Analysis (WD)")

	exam := testZPAExam(100, "Analysis", "IF4B", "WD2B")
	exam.PrimussAncodes = []model.ZPAPrimussAncodes{
		{Program: "IF-B", Ancode: 100},
		{Program: "WD-B", Ancode: -1},
	}
	if err := pg.CacheZPAExams([]*model.ZPAExam{exam}); err != nil {
		t.Fatalf("CacheZPAExams: %v", err)
	}

	if n := count(t, pg, `select count(*) from exam_primuss_ancode where source = 'zpa'`); n != 1 {
		t.Errorf("stored zpa links = %d, want 1 -- the -1 placeholder became a row", n)
	}

	got, err := pg.GetZpaExamByAncode(ctx, 100)
	if err != nil {
		t.Fatalf("GetZpaExamByAncode: %v", err)
	}
	// cleanupPrimussAncodes rebuilds one entry per study group, with -1 where no
	// Primuss ancode is known -- unchanged behaviour.
	if len(got.PrimussAncodes) != 2 {
		t.Fatalf("PrimussAncodes = %+v, want one per group", got.PrimussAncodes)
	}
	byProgram := map[string]int{}
	for _, pa := range got.PrimussAncodes {
		byProgram[pa.Program] = pa.Ancode
	}
	if byProgram["IF-B"] != 100 {
		t.Errorf("IF-B ancode = %d, want 100", byProgram["IF-B"])
	}
	if byProgram["WD-B"] != -1 {
		t.Errorf("WD-B ancode = %d, want -1 (no Primuss exam known)", byProgram["WD-B"])
	}
}

// A manually added mapping is folded in on read and must survive a re-import --
// it is hand-entered and carries source 'added', which the import does not touch.
func TestZPAExamAddedAncodesSurviveAReimport(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")
	seedPrimussExam(t, pg, "IF-B", 555, "Analysis")

	if err := pg.CacheZPAExams([]*model.ZPAExam{testZPAExam(100, "Analysis", "IF4B")}); err != nil {
		t.Fatalf("CacheZPAExams: %v", err)
	}
	if err := pg.AddAncode(ctx, 100, "IF-B", 555); err != nil {
		t.Fatalf("AddAncode: %v", err)
	}

	if err := pg.CacheZPAExams([]*model.ZPAExam{testZPAExam(100, "Analysis", "IF4B")}); err != nil {
		t.Fatalf("CacheZPAExams (second): %v", err)
	}

	got, err := pg.GetZpaExamByAncode(ctx, 100)
	if err != nil {
		t.Fatalf("GetZpaExamByAncode: %v", err)
	}
	found := false
	for _, pa := range got.PrimussAncodes {
		if pa.Program == "IF-B" && pa.Ancode == 555 {
			found = true
		}
	}
	if !found {
		t.Errorf("PrimussAncodes = %+v, want the manually added IF-B/555", got.PrimussAncodes)
	}
}

func TestExamsToPlan(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")
	if err := pg.CacheZPAExams([]*model.ZPAExam{
		testZPAExam(100, "Analysis"),
		testZPAExam(200, "Lineare Algebra"),
		testZPAExam(300, "Programmieren"),
	}); err != nil {
		t.Fatalf("CacheZPAExams: %v", err)
	}

	// Absence of a decision is a real third state: 300 is undecided.
	err := pg.SetZPAExamsToPlan(ctx,
		[]*model.ZPAExam{testZPAExam(100, "Analysis")},
		[]*model.ZPAExam{testZPAExam(200, "Lineare Algebra")})
	if err != nil {
		t.Fatalf("SetZPAExamsToPlan: %v", err)
	}

	toPlan, err := pg.GetZPAExamsToPlan(ctx)
	if err != nil {
		t.Fatalf("GetZPAExamsToPlan: %v", err)
	}
	if len(toPlan) != 1 || toPlan[0].AnCode != 100 {
		t.Errorf("to plan = %+v, want just 100", toPlan)
	}

	notToPlan, err := pg.GetZPAExamsNotToPlan(ctx)
	if err != nil {
		t.Fatalf("GetZPAExamsNotToPlan: %v", err)
	}
	if len(notToPlan) != 1 || notToPlan[0].AnCode != 200 {
		t.Errorf("not to plan = %+v, want just 200", notToPlan)
	}

	decided, err := pg.GetZPAExamsPlannedOrNotPlanned(ctx)
	if err != nil {
		t.Fatalf("GetZPAExamsPlannedOrNotPlanned: %v", err)
	}
	if len(decided) != 2 {
		t.Errorf("decided = %d exams, want 2 -- the undecided one leaked in", len(decided))
	}

	ancodes, err := pg.GetZpaAncodesPlanned(ctx)
	if err != nil {
		t.Fatalf("GetZpaAncodesPlanned: %v", err)
	}
	if (*ancodes).Cardinality() != 1 || !(*ancodes).Contains(100) {
		t.Errorf("planned ancodes = %v, want {100}", (*ancodes).ToSlice())
	}
}

func TestAddAndRemoveZpaExamToPlan(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")
	if err := pg.CacheZPAExams([]*model.ZPAExam{testZPAExam(100, "Analysis")}); err != nil {
		t.Fatalf("CacheZPAExams: %v", err)
	}

	if _, err := pg.AddZpaExamToPlan(ctx, 100); err != nil {
		t.Fatalf("AddZpaExamToPlan: %v", err)
	}
	planned, err := pg.GetZpaAncodesPlanned(ctx)
	if err != nil {
		t.Fatalf("GetZpaAncodesPlanned: %v", err)
	}
	if !(*planned).Contains(100) {
		t.Error("100 is not planned after AddZpaExamToPlan")
	}

	// Flipping it keeps one row: the decision is replaced, not accumulated.
	if _, err := pg.RmZpaExamFromPlan(ctx, 100); err != nil {
		t.Fatalf("RmZpaExamFromPlan: %v", err)
	}
	planned, err = pg.GetZpaAncodesPlanned(ctx)
	if err != nil {
		t.Fatalf("GetZpaAncodesPlanned: %v", err)
	}
	if (*planned).Contains(100) {
		t.Error("100 is still planned after RmZpaExamFromPlan")
	}
	if n := count(t, pg, `select count(*) from exam_to_plan`); n != 1 {
		t.Errorf("exam_to_plan rows = %d, want 1", n)
	}
}

// A decision can only be recorded for an exam that exists -- the foreign key
// retires the "ancode is not a real exam" check ValidateDB did by hand.
func TestExamToPlanNeedsAnExam(t *testing.T) {
	pg := pgtest.NewDB(t)

	seedPrimussFixtures(t, pg, "IF-B")
	if _, err := pg.AddZpaExamToPlan(t.Context(), 999); err == nil {
		t.Error("a planning decision was recorded for an ancode that is not an exam")
	}
}
