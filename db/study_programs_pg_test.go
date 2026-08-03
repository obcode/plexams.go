package db_test

import (
	"strings"
	"testing"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
)

func testStudyProgram(shortname string) *model.StudyProgram {
	return &model.StudyProgram{
		Shortname:         shortname,
		Name:              "Informatik",
		Degree:            strptr("Bachelor"),
		ZpaCode:           "IF",
		Category:          "fk07",
		Active:            true,
		Retired:           false,
		ExternalExamsBase: nil,
		JointFaculty:      nil,
	}
}

func assertStudyProgramEqual(t *testing.T, want, got *model.StudyProgram) {
	t.Helper()

	if got == nil {
		t.Fatal("study program is nil")
	}
	for _, f := range []struct {
		name      string
		want, got string
	}{
		{"Shortname", want.Shortname, got.Shortname},
		{"Name", want.Name, got.Name},
		{"Degree", strval(want.Degree), strval(got.Degree)},
		{"ZpaCode", want.ZpaCode, got.ZpaCode},
		{"Category", want.Category, got.Category},
		{"JointFaculty", strval(want.JointFaculty), strval(got.JointFaculty)},
		{"ExternalExamsBase", intval(want.ExternalExamsBase), intval(got.ExternalExamsBase)},
	} {
		if f.want != f.got {
			t.Errorf("%s = %q, want %q", f.name, f.got, f.want)
		}
	}
	if want.Active != got.Active {
		t.Errorf("Active = %v, want %v", got.Active, want.Active)
	}
	if want.Retired != got.Retired {
		t.Errorf("Retired = %v, want %v", got.Retired, want.Retired)
	}
}

func TestStudyProgramRoundTrip(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	want := testStudyProgram("IF-B")
	if err := pg.UpsertStudyProgram(ctx, want); err != nil {
		t.Fatalf("UpsertStudyProgram: %v", err)
	}

	got, err := pg.StudyProgram(ctx, "IF-B")
	if err != nil {
		t.Fatalf("StudyProgram: %v", err)
	}
	assertStudyProgramEqual(t, want, got)
}

// TestStudyProgramJointRoundTrip covers the shape the live data actually has for
// the three MUC.DAI programs, including the check that ties joint_faculty to the
// category. Every one of the 15 stored programs satisfies it today.
func TestStudyProgramJointRoundTrip(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	want := testStudyProgram("ID-B")
	want.Name = "Data Science and Scientific Computing"
	want.ZpaCode = "ID"
	want.Category = "joint"
	want.JointFaculty = strptr("MUC.DAI")
	want.ExternalExamsBase = intptr(900)

	if err := pg.UpsertStudyProgram(ctx, want); err != nil {
		t.Fatalf("UpsertStudyProgram: %v", err)
	}

	got, err := pg.StudyProgram(ctx, "ID-B")
	if err != nil {
		t.Fatalf("StudyProgram: %v", err)
	}
	assertStudyProgramEqual(t, want, got)
}

// TestStudyProgramEmptyZpaCode pins the one program in the live data (IS-M) that
// has no zpaCode at all: Mongo simply had no such field, and the model documents
// the empty string as "defaults to shortname".
func TestStudyProgramEmptyZpaCode(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	want := testStudyProgram("IS-M")
	want.ZpaCode = ""

	if err := pg.UpsertStudyProgram(ctx, want); err != nil {
		t.Fatalf("UpsertStudyProgram: %v", err)
	}

	got, err := pg.StudyProgram(ctx, "IS-M")
	if err != nil {
		t.Fatalf("StudyProgram: %v", err)
	}
	if got.ZpaCode != "" {
		t.Errorf("ZpaCode = %q, want the empty string", got.ZpaCode)
	}
}

func TestStudyProgramMissingReturnsNilNil(t *testing.T) {
	pg := pgtest.NewDB(t)

	got, err := pg.StudyProgram(t.Context(), "XX-B")
	if err != nil {
		t.Fatalf("StudyProgram: %v", err)
	}
	if got != nil {
		t.Errorf("StudyProgram = %v, want nil", got)
	}
}

func TestStudyProgramUpsertReplaces(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	if err := pg.UpsertStudyProgram(ctx, testStudyProgram("IF-B")); err != nil {
		t.Fatalf("UpsertStudyProgram: %v", err)
	}

	updated := testStudyProgram("IF-B")
	updated.Name = "Informatik (neu)"
	updated.Retired = true
	updated.Degree = nil
	if err := pg.UpsertStudyProgram(ctx, updated); err != nil {
		t.Fatalf("UpsertStudyProgram (second): %v", err)
	}

	got, err := pg.StudyProgram(ctx, "IF-B")
	if err != nil {
		t.Fatalf("StudyProgram: %v", err)
	}
	assertStudyProgramEqual(t, updated, got)

	programs, err := pg.StudyPrograms(ctx)
	if err != nil {
		t.Fatalf("StudyPrograms: %v", err)
	}
	if len(programs) != 1 {
		t.Errorf("len(StudyPrograms) = %d, want 1 -- the upsert inserted instead of replacing", len(programs))
	}
}

func TestStudyProgramsEmptyIsNotNilAndSorted(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	programs, err := pg.StudyPrograms(ctx)
	if err != nil {
		t.Fatalf("StudyPrograms: %v", err)
	}
	if programs == nil {
		t.Fatal("StudyPrograms returned nil, want an empty slice")
	}

	for _, shortname := range []string{"WT-B", "DC-B", "IF-B", "DA-M"} {
		if err := pg.UpsertStudyProgram(ctx, testStudyProgram(shortname)); err != nil {
			t.Fatalf("UpsertStudyProgram(%s): %v", shortname, err)
		}
	}

	programs, err = pg.StudyPrograms(ctx)
	if err != nil {
		t.Fatalf("StudyPrograms: %v", err)
	}
	want := []string{"DA-M", "DC-B", "IF-B", "WT-B"}
	if len(programs) != len(want) {
		t.Fatalf("len(StudyPrograms) = %d, want %d", len(programs), len(want))
	}
	for i, w := range want {
		if programs[i].Shortname != w {
			t.Errorf("StudyPrograms[%d].Shortname = %q, want %q", i, programs[i].Shortname, w)
		}
	}
}

func TestStudyProgramDelete(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	deleted, err := pg.DeleteStudyProgram(ctx, "IF-B")
	if err != nil {
		t.Fatalf("DeleteStudyProgram: %v", err)
	}
	if deleted {
		t.Error("DeleteStudyProgram = true for a program that does not exist")
	}

	if err := pg.UpsertStudyProgram(ctx, testStudyProgram("IF-B")); err != nil {
		t.Fatalf("UpsertStudyProgram: %v", err)
	}

	deleted, err = pg.DeleteStudyProgram(ctx, "IF-B")
	if err != nil {
		t.Fatalf("DeleteStudyProgram: %v", err)
	}
	if !deleted {
		t.Error("DeleteStudyProgram = false for a program that exists")
	}
}

// TestStudyProgramDeleteRefusedWhileReferenced is a deliberate difference to
// Mongo. A vanished program used to leave every exam and registration pointing at
// a code that no longer resolved -- the silent-corruption class this migration
// exists to remove. The error text has to name the program, because it reaches
// the GUI as-is.
func TestStudyProgramDeleteRefusedWhileReferenced(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	if err := pg.UpsertStudyProgram(ctx, testStudyProgram("IF-B")); err != nil {
		t.Fatalf("UpsertStudyProgram: %v", err)
	}
	exec(t, pg, `insert into semester (id, semester, schema_version)
	             values ('2026-WS', '2026 WS', 2)`)
	exec(t, pg, `insert into primuss_exam (semester_id, program, ancode)
	             values ('2026-WS', 'IF-B', 4711)`)

	deleted, err := pg.DeleteStudyProgram(ctx, "IF-B")
	if err == nil {
		t.Fatal("DeleteStudyProgram removed a program that Primuss data still references")
	}
	if deleted {
		t.Error("DeleteStudyProgram = true although it failed")
	}
	if !strings.Contains(err.Error(), "IF-B") {
		t.Errorf("error %q does not name the program", err)
	}

	// The program is still there, and so is the exam referencing it.
	got, err := pg.StudyProgram(ctx, "IF-B")
	if err != nil {
		t.Fatalf("StudyProgram: %v", err)
	}
	if got == nil {
		t.Error("the study program is gone although the delete failed")
	}
	if n := count(t, pg, "select count(*) from primuss_exam"); n != 1 {
		t.Errorf("primuss_exam rows = %d, want 1", n)
	}
}
