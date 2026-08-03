package db_test

import (
	"testing"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
)

func testTeacher(id int, fullname, email string) *model.Teacher {
	return &model.Teacher{
		Shortname:    "obraun",
		Fullname:     fullname,
		IsProf:       true,
		IsLBA:        false,
		IsProfHC:     false,
		IsStaff:      false,
		LastSemester: "2026 WS",
		FK:           "FK07",
		ID:           id,
		Email:        email,
		IsActive:     true,
	}
}

func TestTeacherRoundTrip(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")

	want := testTeacher(42, "Prof. Dr. Oliver Braun", "Oliver.Braun@hm.edu")
	if err := pg.CacheTeachers([]*model.Teacher{want}, "2026 WS"); err != nil {
		t.Fatalf("CacheTeachers: %v", err)
	}

	got, err := pg.GetTeacher(ctx, 42)
	if err != nil {
		t.Fatalf("GetTeacher: %v", err)
	}
	if got.Fullname != want.Fullname || got.Shortname != want.Shortname || got.Email != want.Email {
		t.Errorf("teacher = %+v, want %+v", got, want)
	}
	if got.FK != "FK07" || !got.IsProf || !got.IsActive {
		t.Errorf("flags = fk %q, prof %v, active %v", got.FK, got.IsProf, got.IsActive)
	}
}

// Id 0 is "nobody" -- exams without a main examer id rely on getting an empty
// teacher rather than an error.
func TestTeacherZeroIDIsNobody(t *testing.T) {
	pg := pgtest.NewDB(t)

	got, err := pg.GetTeacher(t.Context(), 0)
	if err != nil {
		t.Fatalf("GetTeacher(0): %v", err)
	}
	if got == nil || got.ID != 0 || got.Fullname != "" {
		t.Errorf("GetTeacher(0) = %+v, want an empty teacher", got)
	}
}

// ZPA stores raw addresses and our user emails are lower-cased, so the lookup has
// to be case-insensitive -- otherwise a planner logs in and is not recognised as
// a teacher. A miss is nil, not an error: the auth code reads it as "not a
// teacher", not as a failure.
func TestTeacherByEmailIsCaseInsensitive(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")
	if err := pg.CacheTeachers([]*model.Teacher{
		testTeacher(42, "Prof. Dr. Oliver Braun", "Oliver.Braun@hm.edu"),
	}, "2026 WS"); err != nil {
		t.Fatalf("CacheTeachers: %v", err)
	}

	for _, email := range []string{"oliver.braun@hm.edu", "Oliver.Braun@hm.edu", "OLIVER.BRAUN@HM.EDU"} {
		got, err := pg.GetTeacherByEmail(ctx, email)
		if err != nil {
			t.Fatalf("GetTeacherByEmail(%s): %v", email, err)
		}
		if got == nil {
			t.Errorf("GetTeacherByEmail(%s) = nil, want the teacher", email)
			continue
		}
		if got.ID != 42 {
			t.Errorf("GetTeacherByEmail(%s).ID = %d, want 42", email, got.ID)
		}
	}

	got, err := pg.GetTeacherByEmail(ctx, "niemand@hm.edu")
	if err != nil {
		t.Fatalf("GetTeacherByEmail: %v", err)
	}
	if got != nil {
		t.Errorf("GetTeacherByEmail = %+v, want nil", got)
	}
}

func TestTeacherByNameRegex(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")
	if err := pg.CacheTeachers([]*model.Teacher{
		testTeacher(42, "Prof. Dr. Oliver Braun", "oliver.braun@hm.edu"),
		testTeacher(43, "Prof. Dr. Veronika Thurner", "veronika.thurner@hm.edu"),
	}, "2026 WS"); err != nil {
		t.Fatalf("CacheTeachers: %v", err)
	}

	id, err := pg.GetTeacherIdByRegex(ctx, "Thurner")
	if err != nil {
		t.Fatalf("GetTeacherIdByRegex: %v", err)
	}
	if id != 43 {
		t.Errorf("id = %d, want 43", id)
	}

	if _, err := pg.GetTeacherIdByRegex(ctx, "Niemand"); err == nil {
		t.Error("GetTeacherIdByRegex found a teacher that does not exist")
	}
}

// GetInvigilators applies isInvigilator, the single definition of who counts --
// shared with the Mongo layer rather than reimplemented as a WHERE clause.
func TestInvigilatorsAreFilteredByTheSharedRule(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")

	prof := testTeacher(1, "Eine Professorin", "a@hm.edu")
	lba := testTeacher(2, "Ein LBA", "b@hm.edu")
	lba.IsLBA = true
	profHC := testTeacher(3, "Ein Honorarprofessor", "c@hm.edu")
	profHC.IsProfHC = true
	otherFK := testTeacher(4, "Jemand aus der FK08", "d@hm.edu")
	otherFK.FK = "FK08"
	staff := testTeacher(5, "Jemand aus der Verwaltung", "e@hm.edu")
	staff.IsProf = false

	if err := pg.CacheTeachers([]*model.Teacher{prof, lba, profHC, otherFK, staff}, "2026 WS"); err != nil {
		t.Fatalf("CacheTeachers: %v", err)
	}

	all, err := pg.GetTeachers(ctx)
	if err != nil {
		t.Fatalf("GetTeachers: %v", err)
	}
	if len(all) != 5 {
		t.Errorf("len(GetTeachers) = %d, want 5", len(all))
	}

	invigilators, err := pg.GetInvigilators(ctx)
	if err != nil {
		t.Fatalf("GetInvigilators: %v", err)
	}
	if len(invigilators) != 1 || invigilators[0].ID != 1 {
		t.Errorf("invigilators = %+v, want just the FK07 professor", invigilators)
	}
}

// CacheTeachers really does clear and refill, unlike the exams: nothing
// references a teacher by foreign key, so there is no planner work to lose, and
// someone who left should leave the invigilator pool rather than linger.
func TestCacheTeachersReplaces(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")
	if err := pg.CacheTeachers([]*model.Teacher{
		testTeacher(1, "Bleibt", "a@hm.edu"),
		testTeacher(2, "Geht", "b@hm.edu"),
	}, "2026 WS"); err != nil {
		t.Fatalf("CacheTeachers: %v", err)
	}
	if err := pg.CacheTeachers([]*model.Teacher{
		testTeacher(1, "Bleibt", "a@hm.edu"),
	}, "2026 WS"); err != nil {
		t.Fatalf("CacheTeachers (second): %v", err)
	}

	all, err := pg.GetTeachers(ctx)
	if err != nil {
		t.Fatalf("GetTeachers: %v", err)
	}
	if len(all) != 1 || all[0].ID != 1 {
		t.Errorf("teachers = %+v, want just the one that is still delivered", all)
	}

	// An empty import is a real case and must not error.
	if err := pg.CacheTeachers(nil, "2026 WS"); err != nil {
		t.Fatalf("CacheTeachers (empty): %v", err)
	}
	all, err = pg.GetTeachers(ctx)
	if err != nil {
		t.Fatalf("GetTeachers: %v", err)
	}
	if all == nil {
		t.Fatal("GetTeachers returned nil, want an empty slice")
	}
	if len(all) != 0 {
		t.Errorf("len = %d, want 0", len(all))
	}
}

func TestZPAStudents(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")
	exec(t, pg, `insert into zpa_student (semester_id, mtknr, greeting, first_name, last_name, email, gender, group_name)
	             values ('2026-WS', '00012345', 'Frau', 'Andrea', 'Beispiel', 'a@hm.edu', 'w', 'IF4B'),
	                    ('2026-WS', '00000001', 'Herr', 'Bernd',  'Anders',   'b@hm.edu', 'm', 'IF4B')`)

	students, err := pg.GetZPAStudents(ctx)
	if err != nil {
		t.Fatalf("GetZPAStudents: %v", err)
	}
	if len(students) != 2 || students[0].LastName != "Anders" {
		t.Errorf("students = %+v, want Anders first", students)
	}

	got, err := pg.GetZPAStudentByMtknr(ctx, "00012345")
	if err != nil {
		t.Fatalf("GetZPAStudentByMtknr: %v", err)
	}
	if got.FirstName != "Andrea" || got.Group != "IF4B" || got.Greeting != "Frau" {
		t.Errorf("student = %+v", got)
	}

	if _, err := pg.GetZPAStudentByMtknr(ctx, "00009999"); err == nil {
		t.Error("GetZPAStudentByMtknr found a student that does not exist")
	}
}
