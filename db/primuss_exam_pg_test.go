package db_test

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/internal/pgtest"
)

// seedPrimussFixtures creates the semester and the study programs the Primuss
// tests reference. The programs are the internal, possibly degree-suffixed
// shortnames -- primuss_exam.program references study_program(shortname).
//
// zpa_code is the leading two letters, which is what the live master data holds
// (IF-B carries "IF"): the program resolver maps a ZPA study group like "IF4B"
// through it, so a fixture without it would silently exercise the fallback path
// for old un-suffixed semesters instead of the real one.
func seedPrimussFixtures(t *testing.T, pg *db.PG, programs ...string) {
	t.Helper()

	exec(t, pg, `insert into semester (id, semester, schema_version)
	             values ('2026-WS', '2026 WS', 2)`)
	for _, program := range programs {
		zpaCode := program
		if len(zpaCode) > 2 {
			zpaCode = zpaCode[:2]
		}
		exec(t, pg, `insert into study_program (shortname, name, zpa_code, category)
		             values ($1, $1, $2, 'fk07')`, program, zpaCode)
	}
}

func seedPrimussExam(t *testing.T, pg *db.PG, program string, ancode int, module string) {
	t.Helper()
	exec(t, pg, `insert into primuss_exam (semester_id, program, ancode, module, main_examer, exam_type, presence)
	             values ('2026-WS', $1, $2, $3, 'Braun O.', 'Klausur', 'P')`, program, ancode, module)
}

func TestPrimussExamRoundTrip(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")
	seedPrimussExam(t, pg, "IF-B", 100, "Analysis")

	got, err := pg.GetPrimussExam(ctx, "IF-B", 100)
	if err != nil {
		t.Fatalf("GetPrimussExam: %v", err)
	}
	if got.AnCode != 100 {
		t.Errorf("AnCode = %d, want 100", got.AnCode)
	}
	if got.Module != "Analysis" {
		t.Errorf("Module = %q, want %q", got.Module, "Analysis")
	}
	if got.Program != "IF-B" {
		t.Errorf("Program = %q, want %q", got.Program, "IF-B")
	}
	if got.MainExamer != "Braun O." {
		t.Errorf("MainExamer = %q, want %q", got.MainExamer, "Braun O.")
	}
	if got.ExamType != "Klausur" {
		t.Errorf("ExamType = %q, want %q", got.ExamType, "Klausur")
	}
	if got.Presence != "P" {
		t.Errorf("Presence = %q, want %q", got.Presence, "P")
	}
}

// A missing Primuss exam stays an error rather than (nil, nil): the connected-exam
// code asks every program for an ancode and reads the error as "not in this one".
func TestPrimussExamMissingIsAnError(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")

	_, err := pg.GetPrimussExam(ctx, "IF-B", 999)
	if err == nil {
		t.Fatal("GetPrimussExam of a missing exam returned no error")
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("err = %v, want pgx.ErrNoRows", err)
	}

	exists, err := pg.PrimussExamExists(ctx, "IF-B", 999)
	if err != nil {
		t.Fatalf("PrimussExamExists: %v", err)
	}
	if exists {
		t.Error("PrimussExamExists = true for an exam that does not exist")
	}
}

func TestPrimussExamsForAncodeAcrossPrograms(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B", "WD-B", "IT-M")
	seedPrimussExam(t, pg, "WD-B", 100, "Analysis (WD)")
	seedPrimussExam(t, pg, "IF-B", 100, "Analysis (IF)")
	seedPrimussExam(t, pg, "IT-M", 200, "Etwas anderes")

	exams, err := pg.GetPrimussExamsForAncode(ctx, 100)
	if err != nil {
		t.Fatalf("GetPrimussExamsForAncode: %v", err)
	}
	if len(exams) != 2 {
		t.Fatalf("len = %d, want 2", len(exams))
	}
	if exams[0].Program != "IF-B" || exams[1].Program != "WD-B" {
		t.Errorf("programs = %s, %s -- want IF-B, WD-B in that order",
			exams[0].Program, exams[1].Program)
	}
}

// GetPrograms is "which programs have exams". Under Mongo it was a regex over
// collection names, which is why an emptied collection kept its program visible
// forever -- and why DropPrimussData had to drop rather than empty.
func TestPrimussProgramsFollowTheExams(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B", "WD-B", "IT-M")

	programs, err := pg.GetPrograms(ctx)
	if err != nil {
		t.Fatalf("GetPrograms: %v", err)
	}
	if len(programs) != 0 {
		t.Errorf("programs = %v, want none -- a study program without exams is not a Primuss program", programs)
	}

	seedPrimussExam(t, pg, "WD-B", 100, "Modul")
	seedPrimussExam(t, pg, "IF-B", 100, "Modul")

	programs, err = pg.GetPrograms(ctx)
	if err != nil {
		t.Fatalf("GetPrograms: %v", err)
	}
	if len(programs) != 2 || programs[0] != "IF-B" || programs[1] != "WD-B" {
		t.Errorf("programs = %v, want [IF-B WD-B]", programs)
	}
}

func TestDropPrimussDataLeavesTheAncodeOverlay(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")
	seedPrimussExam(t, pg, "IF-B", 100, "Analysis")
	exec(t, pg, `insert into primuss_count (semester_id, program, ancode, total)
	             values ('2026-WS', 'IF-B', 100, 3)`)
	exec(t, pg, `insert into studentreg (semester_id, program, student_program, primuss_ancode, mtknr)
	             values ('2026-WS', 'IF-B', 'IF-B', 100, '00012345')`)
	// The hand-entered overlay hangs off the ZPA exam, not off the Primuss data.
	exec(t, pg, `insert into exam (semester_id, ancode, source, module, main_examer, main_examer_id,
	                               exam_type, exam_type_full, duration_min)
	             values ('2026-WS', 100, 'zpa', 'Analysis', 'Braun', 1, 'schriftlich', 'schriftliche Prüfung', 90)`)
	if err := pg.AddAncode(ctx, 100, "IF-B", 100); err != nil {
		t.Fatalf("AddAncode: %v", err)
	}

	programs, err := pg.DropPrimussData(ctx)
	if err != nil {
		t.Fatalf("DropPrimussData: %v", err)
	}
	if len(programs) != 1 || programs[0] != "IF-B" {
		t.Errorf("programs = %v, want [IF-B]", programs)
	}

	for _, table := range []string{"primuss_exam", "primuss_count", "studentreg"} {
		if n := count(t, pg, "select count(*) from "+table); n != 0 {
			t.Errorf("%s still has %d rows after the drop", table, n)
		}
	}
	// The manually added mapping is not re-imported and must survive.
	if n := count(t, pg, "select count(*) from exam_primuss_ancode"); n != 1 {
		t.Errorf("exam_primuss_ancode rows = %d, want 1 -- the drop ate hand-entered data", n)
	}

	// ... and the program is gone from discovery, which emptying a Mongo
	// collection would not have achieved.
	programs, err = pg.GetPrograms(ctx)
	if err != nil {
		t.Fatalf("GetPrograms: %v", err)
	}
	if len(programs) != 0 {
		t.Errorf("programs = %v after the drop, want none", programs)
	}
}
