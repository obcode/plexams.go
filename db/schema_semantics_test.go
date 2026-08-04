package db_test

import (
	"testing"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/internal/pgtest"
)

// seedExamFixtures creates a semester, a study program and the given ZPA exams.
func seedExamFixtures(t *testing.T, pg *db.PG, ancodes ...int) {
	t.Helper()
	ctx := t.Context()

	exec(t, pg, `insert into semester (id, schema_version) values ('2026-WS', 2)`)
	exec(t, pg, `insert into study_program (shortname, name, category)
	             values ('IF-B', 'Informatik', 'fk07')`)
	for _, ancode := range ancodes {
		if _, err := pg.PoolForTest().Exec(ctx, `
			insert into exam (semester_id, ancode, source, module, main_examer,
			                  main_examer_id, exam_type, exam_type_full, duration_min)
			values ('2026-WS', $1, 'zpa', 'Modul', 'Braun', 1, 'schriftlich', 'schriftliche Prüfung', 90)`,
			ancode); err != nil {
			t.Fatalf("seed exam %d: %v", ancode, err)
		}
	}
}

func exec(t *testing.T, pg *db.PG, sql string, args ...any) {
	t.Helper()
	if _, err := pg.PoolForTest().Exec(t.Context(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func count(t *testing.T, pg *db.PG, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := pg.PoolForTest().QueryRow(t.Context(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return n
}

// TestZPAReimportPreservesPlannerOverlay pins the behaviour of the everyday case:
// a re-import in which two exams appear and one disappears.
//
// The exams come from ZPA and can be re-imported at will. The constraints hanging
// off them are typed in by hand and cannot. So an exam ZPA stops delivering is
// MARKED (withdrawn_at), never deleted -- otherwise one flaky import would cascade
// away work that only exists in someone's head. Under MongoDB those overlay
// documents survived as orphans and ValidateDBReferences reported them; this keeps
// them and additionally records why.
func TestZPAReimportPreservesPlannerOverlay(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	// Before: three exams, and the planner has done real work on 300.
	seedExamFixtures(t, pg, 100, 200, 300)
	exec(t, pg, `insert into exam_constraint (semester_id, ancode, online, exclude_days)
	             values ('2026-WS', 300, true, array['2027-01-15 00:00+01'::timestamptz])`)
	exec(t, pg, `insert into exam_room_constraint (semester_id, ancode, exahm) values ('2026-WS', 300, true)`)
	exec(t, pg, `insert into exam_duration_override (semester_id, ancode, duration_min) values ('2026-WS', 300, 120)`)
	exec(t, pg, `insert into exam_to_plan (semester_id, ancode, to_plan) values ('2026-WS', 300, true)`)

	// The re-import: 100 and 200 come again (upsert, module changed on 100),
	// 400 and 500 are new, 300 is gone from ZPA.
	exec(t, pg, `
		insert into exam (semester_id, ancode, source, module, main_examer, main_examer_id,
		                  exam_type, exam_type_full, duration_min)
		values ('2026-WS', 100, 'zpa', 'Analysis (neu)', 'Braun', 1, 'schriftlich', 'schriftliche Prüfung', 90),
		       ('2026-WS', 200, 'zpa', 'Modul',          'Braun', 1, 'schriftlich', 'schriftliche Prüfung', 90),
		       ('2026-WS', 400, 'zpa', 'Neu A',          'Braun', 1, 'schriftlich', 'schriftliche Prüfung', 90),
		       ('2026-WS', 500, 'zpa', 'Neu B',          'Braun', 1, 'schriftlich', 'schriftliche Prüfung', 90)
		on conflict (semester_id, ancode) do update set
		    module = excluded.module, duration_min = excluded.duration_min,
		    withdrawn_at = null`)
	exec(t, pg, `
		update exam set withdrawn_at = now()
		where semester_id = '2026-WS' and source = 'zpa'
		  and ancode <> all($1::int[])`, []int{100, 200, 400, 500})

	// The two new ones arrived, the update took, nothing was lost.
	if n := count(t, pg, `select count(*) from exam where semester_id='2026-WS'`); n != 5 {
		t.Errorf("exam count = %d, want 5", n)
	}
	if n := count(t, pg, `select count(*) from exam
	                      where semester_id='2026-WS' and withdrawn_at is null`); n != 4 {
		t.Errorf("active exam count = %d, want 4", n)
	}
	var module string
	if err := pg.PoolForTest().QueryRow(ctx,
		`select module from exam where semester_id='2026-WS' and ancode=100`).Scan(&module); err != nil {
		t.Fatalf("read module: %v", err)
	}
	if module != "Analysis (neu)" {
		t.Errorf("module = %q, want the upserted value", module)
	}

	// The withdrawn exam is marked, not gone ...
	if n := count(t, pg, `select count(*) from exam
	                      where semester_id='2026-WS' and ancode=300 and withdrawn_at is not null`); n != 1 {
		t.Error("exam 300 should be marked withdrawn, not deleted")
	}

	// ... and every piece of hand-entered work on it survived.
	for _, table := range []string{
		"exam_constraint", "exam_room_constraint", "exam_duration_override", "exam_to_plan",
	} {
		if n := count(t, pg,
			`select count(*) from `+table+` where semester_id='2026-WS' and ancode=300`); n != 1 {
			t.Errorf("%s for the withdrawn exam is gone -- the import destroyed planner input", table)
		}
	}
}

// TestMovingAnExamCannotStaleTheRoomPlan is the point of the whole migration in
// one test.
//
// rooms_planned used to carry its own copy of the exam's start time, and nothing
// kept it in sync: SetExamTime (plexams/plan.go:29) writes the plan entry and
// never touches the room plan. Moving an exam left every one of its rooms
// pointing at the old time, and a hand-written detector looked for the damage
// afterwards (plexams/validate_db.go:261). The column no longer exists.
func TestMovingAnExamCannotStaleTheRoomPlan(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg, 225)
	exec(t, pg, `insert into room (name, seats) values ('R1.046', 60)`)
	exec(t, pg, `insert into plan_entry (semester_id, ancode, starttime)
	             values ('2026-WS', 225, '2027-01-20 08:30+01')`)
	exec(t, pg, `insert into planned_room (semester_id, ancode, room_name, duration_min)
	             values ('2026-WS', 225, 'R1.046', 90)`)

	readRoomTime := func() string {
		t.Helper()
		var wall string
		if err := pg.PoolForTest().QueryRow(ctx,
			`select to_char(starttime, 'DD.MM.YYYY HH24:MI') from planned_room_v
			 where semester_id='2026-WS' and ancode=225`).Scan(&wall); err != nil {
			t.Fatalf("read planned_room_v: %v", err)
		}
		return wall
	}

	if got := readRoomTime(); got != "20.01.2027 08:30" {
		t.Fatalf("room time = %q before the move", got)
	}

	// Move the exam. Only the plan entry is touched -- exactly what SetExamTime
	// does today, and exactly what used to leave the room plan behind.
	exec(t, pg, `update plan_entry set starttime = '2027-01-22 14:00+01'
	             where semester_id='2026-WS' and ancode=225`)

	if got := readRoomTime(); got != "22.01.2027 14:00" {
		t.Errorf("room time = %q after the move, want the new time -- the room plan went stale", got)
	}
}

// TestOneRoomHoldsSeveralNTABookings pins the natural key of planned_room, which
// is (ancode, room, nta_mtknr) and NOT (ancode, room).
//
// One physical room legitimately holds several bookings for the same exam,
// because every NTA student sitting there needs their own row for their own
// extended duration -- ancode 225 in R1.046 in 2026-SS is 43 students at 90
// minutes plus three single students at 99. What must still be impossible is the
// same booking twice.
func TestOneRoomHoldsSeveralNTABookings(t *testing.T) {
	pg := pgtest.NewDB(t)

	seedExamFixtures(t, pg, 225)
	exec(t, pg, `insert into room (name, seats) values ('R1.046', 60)`)
	exec(t, pg, `insert into nta (mtknr, name, compensation, delta_duration_percent,
	                              program, valid_from, valid_until)
	             values ('39644321', 'A', 'Zeitverlängerung', 10, 'IF-B', '2026-WS', '2027-SS'),
	                    ('21384524', 'B', 'Zeitverlängerung', 10, 'IF-B', '2026-WS', '2027-SS')`)
	exec(t, pg, `insert into plan_entry (semester_id, ancode, starttime)
	             values ('2026-WS', 225, '2027-01-20 08:30+01')`)

	exec(t, pg, `insert into planned_room (semester_id, ancode, room_name, duration_min)
	             values ('2026-WS', 225, 'R1.046', 90)`)
	exec(t, pg, `insert into planned_room (semester_id, ancode, room_name, duration_min, handicap, nta_mtknr)
	             values ('2026-WS', 225, 'R1.046', 99, true, '39644321'),
	                    ('2026-WS', 225, 'R1.046', 99, true, '21384524')`)

	if n := count(t, pg, `select count(*) from planned_room where semester_id='2026-WS'`); n != 3 {
		t.Fatalf("planned_room count = %d, want 3", n)
	}

	// The ordinary booking a second time must fail -- which only works because the
	// unique constraint is NULLS NOT DISTINCT. PostgreSQL's default would treat
	// every NULL nta_mtknr as unique and let this through.
	if _, err := pg.PoolForTest().Exec(t.Context(),
		`insert into planned_room (semester_id, ancode, room_name, duration_min)
		 values ('2026-WS', 225, 'R1.046', 90)`); err == nil {
		t.Error("a duplicate non-NTA booking was accepted -- NULLS NOT DISTINCT is missing")
	}
}

// TestDeletingAnExamCascades is the other half of the contract, and the reason the
// import must not delete: a real DELETE does take the overlay with it. That is
// correct for a deliberate act (dropping a semester, removing an external exam the
// planner created) and catastrophic for an import.
func TestDeletingAnExamCascades(t *testing.T) {
	pg := pgtest.NewDB(t)

	seedExamFixtures(t, pg, 300)
	exec(t, pg, `insert into exam_constraint (semester_id, ancode, online) values ('2026-WS', 300, true)`)

	exec(t, pg, `delete from exam where semester_id='2026-WS' and ancode=300`)

	if n := count(t, pg, `select count(*) from exam_constraint where semester_id='2026-WS'`); n != 0 {
		t.Errorf("constraint count = %d, want 0 -- the cascade did not fire", n)
	}
}

// TestSemesterDeletionCascades: dropping a semester must take its whole contents
// with it, since nothing outside it can reference them.
func TestSemesterDeletionCascades(t *testing.T) {
	pg := pgtest.NewDB(t)

	seedExamFixtures(t, pg, 100)
	exec(t, pg, `insert into exam_constraint (semester_id, ancode, online) values ('2026-WS', 100, true)`)
	exec(t, pg, `insert into semester_config_input (semester_id, config) values ('2026-WS', '{}')`)

	exec(t, pg, `delete from semester where id='2026-WS'`)

	for _, table := range []string{"exam", "exam_constraint", "semester_config_input"} {
		if n := count(t, pg, `select count(*) from `+table); n != 0 {
			t.Errorf("%s still has %d rows after the semester was deleted", table, n)
		}
	}
}

// TestStudentRegKeepsBothPrograms pins the distinction the live data forced into
// the schema.
//
// Under MongoDB one program was the collection name (studentregs_IF) and the
// other a field in the document (Stg), so nothing pushed them together. A single
// `program` column would have merged them, and the merge would have been
// invisible: both are strings, both are called Program, and 98 % of the rows
// agree. The 2 % that do not are precisely the cross-program registrations that
// the NTA program check, model.Student.Program and the FK07 statistic exist for.
func TestStudentRegKeepsBothPrograms(t *testing.T) {
	pg := pgtest.NewDB(t)

	exec(t, pg, `insert into semester (id, schema_version) values ('2026-WS', 2)`)
	exec(t, pg, `insert into study_program (shortname, name, category)
	             values ('IF-B', 'Informatik', 'fk07')`)

	// A student of another faculty's programme registered for one of our exams:
	// the exam is IF-B, the student is an EI (Elektrotechnik) one.
	exec(t, pg, `insert into studentreg (semester_id, program, student_program, primuss_ancode, mtknr, name)
	             values ('2026-WS', 'IF-B', 'EI', 100, '00012345', 'Eine Person')`)

	var examProgram, studentProgram string
	if err := pg.PoolForTest().QueryRow(t.Context(),
		`select program, student_program from studentreg where mtknr = '00012345'`).
		Scan(&examProgram, &studentProgram); err != nil {
		t.Fatalf("read programs: %v", err)
	}
	if examProgram != "IF-B" {
		t.Errorf("program = %q, want the exam's program IF-B", examProgram)
	}
	if studentProgram != "EI" {
		t.Errorf("student_program = %q, want the student's own programme EI", studentProgram)
	}
}

// TestStudentProgramNeedsNoStudyProgramRow is the other half: student_program
// must NOT reference study_program. 14 of its values in 2026-SS are other
// faculties' codes, and a foreign key there would reject the import rather than
// protect anything -- the same reasoning as the missing unique index on
// (ancode, mtknr).
func TestStudentProgramNeedsNoStudyProgramRow(t *testing.T) {
	pg := pgtest.NewDB(t)

	exec(t, pg, `insert into semester (id, schema_version) values ('2026-WS', 2)`)
	exec(t, pg, `insert into study_program (shortname, name, category)
	             values ('IF-B', 'Informatik', 'fk07')`)

	for _, foreign := range []string{"AR", "BW", "CH", "DF", "DS", "DT", "EI", "GD", "ME", "MN", "PN", "RS", "TP", "WI"} {
		if _, err := pg.PoolForTest().Exec(t.Context(),
			`insert into studentreg (semester_id, program, student_program, primuss_ancode, mtknr)
			 values ('2026-WS', 'IF-B', $1, 100, '00012345')`, foreign); err != nil {
			t.Errorf("a registration from programme %s was rejected: %v", foreign, err)
		}
	}
}
