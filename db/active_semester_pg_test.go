package db_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/internal/pgtest"
)

// pgtest connects with the semester "2026-WS", so the registry row has to carry
// that id for the foreign key to resolve.
func TestActiveSemesterRoundTrip(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	exec(t, pg, `insert into semester (id, schema_version) values ('2026-WS', 2)`)

	if err := pg.SaveActiveSemester(ctx); err != nil {
		t.Fatalf("SaveActiveSemester: %v", err)
	}

	got, err := pg.GetActiveSemester(ctx)
	if err != nil {
		t.Fatalf("GetActiveSemester: %v", err)
	}
	if got == nil {
		t.Fatal("active semester is nil")
	}
	if got.Semester != "2026-WS" {
		t.Errorf("Semester = %q, want %q", got.Semester, "2026-WS")
	}
}

func TestActiveSemesterMissingReturnsNilNil(t *testing.T) {
	pg := pgtest.NewDB(t)

	got, err := pg.GetActiveSemester(t.Context())
	if err != nil {
		t.Fatalf("GetActiveSemester: %v", err)
	}
	if got != nil {
		t.Errorf("GetActiveSemester = %v, want nil", got)
	}
}

// TestActiveSemesterUnregisteredSemesterIsRejected pins the difference to Mongo:
// the document there could name a database that had since been dropped, and the
// next start would try to resume into it.
//
// It is reported as ErrSemesterNotRegistered, not as a generic write failure:
// every first boot against an empty database hits this path, because --semester
// pins a semester so that createSemester can be called at all. Telling the two
// apart is what keeps a normal bootstrap from logging errors.
func TestActiveSemesterUnregisteredSemesterIsRejected(t *testing.T) {
	pg := pgtest.NewDB(t)

	err := pg.SaveActiveSemester(t.Context())
	if err == nil {
		t.Fatal("SaveActiveSemester accepted a semester that is not in the registry")
	}
	if !errors.Is(err, db.ErrSemesterNotRegistered) {
		t.Errorf("SaveActiveSemester = %v, want ErrSemesterNotRegistered", err)
	}
	if !strings.Contains(err.Error(), "2026-WS") {
		t.Errorf("error %q does not name the semester it refused", err)
	}
}

func TestActiveSemesterIsASingleton(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	exec(t, pg, `insert into semester (id, schema_version)
	             values ('2026-WS', 2), ('2026-SS', 2)`)

	if err := pg.SaveActiveSemester(ctx); err != nil {
		t.Fatalf("SaveActiveSemester: %v", err)
	}
	if err := pg.SaveActiveSemester(ctx); err != nil {
		t.Fatalf("SaveActiveSemester (second): %v", err)
	}

	if n := count(t, pg, "select count(*) from active_semester"); n != 1 {
		t.Errorf("active_semester rows = %d, want 1", n)
	}
}
