package db_test

import (
	"testing"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
)

func student(mtknr string, withNta bool) *model.Student {
	s := &model.Student{Mtknr: mtknr, Program: "IF-B", Name: "Eine Person"}
	if withNta {
		s.Nta = testNta(mtknr, "Eine Person")
	}
	return s
}

// TestSetSemesterOnNTAsWritesTheLogicalLabel is the finding that came out of the
// live data: all 63 stored lastSemester values are the logical semester
// ("2026 SS"), not the workspace id ("2026-WS"). PG carries only the workspace
// id, so the label has to be resolved through the registry -- writing
// db.semesterID here would have corrupted the field on the first prepare run,
// and nothing in the GUI would have looked wrong.
func TestSetSemesterOnNTAsWritesTheLogicalLabel(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	exec(t, pg, `insert into semester (id, semester, schema_version)
	             values ('2026-WS', '2026 WS', 2)`)
	for _, mtknr := range []string{"00000001", "00000002"} {
		if _, err := pg.AddNta(ctx, testNta(mtknr, "Person "+mtknr)); err != nil {
			t.Fatalf("AddNta(%s): %v", mtknr, err)
		}
	}

	regs := []interface{}{
		student("00000001", true),
		student("00000002", true),
	}
	if err := pg.SetSemesterOnNTAs(ctx, regs); err != nil {
		t.Fatalf("SetSemesterOnNTAs: %v", err)
	}

	for _, mtknr := range []string{"00000001", "00000002"} {
		nta, err := pg.Nta(ctx, mtknr)
		if err != nil {
			t.Fatalf("Nta(%s): %v", mtknr, err)
		}
		if strval(nta.LastSemester) != "2026 WS" {
			t.Errorf("%s: LastSemester = %s, want %q (the logical semester, not the workspace id)",
				mtknr, strval(nta.LastSemester), "2026 WS")
		}
	}
}

// Students without a compensation are skipped, exactly as before -- they have no
// NTA row to stamp.
func TestSetSemesterOnNTAsSkipsStudentsWithoutNta(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	exec(t, pg, `insert into semester (id, semester, schema_version)
	             values ('2026-WS', '2026 WS', 2)`)
	if _, err := pg.AddNta(ctx, testNta("00000001", "Mit Nachteilsausgleich")); err != nil {
		t.Fatalf("AddNta: %v", err)
	}
	if _, err := pg.AddNta(ctx, testNta("00000002", "Nicht gemeldet")); err != nil {
		t.Fatalf("AddNta: %v", err)
	}

	regs := []interface{}{
		student("00000001", true),
		student("00000002", false),
	}
	if err := pg.SetSemesterOnNTAs(ctx, regs); err != nil {
		t.Fatalf("SetSemesterOnNTAs: %v", err)
	}

	stamped, err := pg.Nta(ctx, "00000001")
	if err != nil {
		t.Fatalf("Nta: %v", err)
	}
	if strval(stamped.LastSemester) != "2026 WS" {
		t.Errorf("LastSemester = %s, want %q", strval(stamped.LastSemester), "2026 WS")
	}

	untouched, err := pg.Nta(ctx, "00000002")
	if err != nil {
		t.Fatalf("Nta: %v", err)
	}
	if untouched.LastSemester != nil {
		t.Errorf("LastSemester = %q, want nil -- the student has no compensation this semester",
			*untouched.LastSemester)
	}
}

// A student whose NTA row is gone must not abort the run: the Mongo version
// logged it per student and carried on, and prepare runs over the whole cohort.
func TestSetSemesterOnNTAsToleratesAMissingNta(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	exec(t, pg, `insert into semester (id, semester, schema_version)
	             values ('2026-WS', '2026 WS', 2)`)
	if _, err := pg.AddNta(ctx, testNta("00000001", "Vorhanden")); err != nil {
		t.Fatalf("AddNta: %v", err)
	}

	regs := []interface{}{
		student("00000001", true),
		student("00009999", true), // no nta row
	}
	if err := pg.SetSemesterOnNTAs(ctx, regs); err != nil {
		t.Fatalf("SetSemesterOnNTAs: %v", err)
	}

	got, err := pg.Nta(ctx, "00000001")
	if err != nil {
		t.Fatalf("Nta: %v", err)
	}
	if strval(got.LastSemester) != "2026 WS" {
		t.Errorf("LastSemester = %s, want %q", strval(got.LastSemester), "2026 WS")
	}
}

func TestSetSemesterOnNTAsWithoutRegistrations(t *testing.T) {
	pg := pgtest.NewDB(t)

	// No registry row either: with nothing to stamp there is nothing to resolve.
	if err := pg.SetSemesterOnNTAs(t.Context(), nil); err != nil {
		t.Fatalf("SetSemesterOnNTAs: %v", err)
	}
}

// A workspace missing from the registry has to be loud. Mongo wrote db.semester
// unconditionally; here there is no label to write, and silently stamping
// nothing would look like a successful prepare.
func TestSetSemesterOnNTAsUnknownWorkspaceFails(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	if _, err := pg.AddNta(ctx, testNta("00000001", "Vorhanden")); err != nil {
		t.Fatalf("AddNta: %v", err)
	}

	err := pg.SetSemesterOnNTAs(ctx, []interface{}{student("00000001", true)})
	if err == nil {
		t.Fatal("SetSemesterOnNTAs succeeded although the workspace is not in the registry")
	}

	got, err := pg.Nta(ctx, "00000001")
	if err != nil {
		t.Fatalf("Nta: %v", err)
	}
	if got.LastSemester != nil {
		t.Errorf("LastSemester = %q, want nil", *got.LastSemester)
	}
}
