package db

import (
	"testing"
)

// seedExamFixtures creates a semester, a study program and the given ZPA exams.
func seedExamFixtures(t *testing.T, db *PG, ancodes ...int) {
	t.Helper()
	ctx := t.Context()

	exec(t, db, `insert into semester (id, semester, schema_version)
	             values ('2026-WS', '2026 WS', 2)`)
	exec(t, db, `insert into study_program (shortname, name, category)
	             values ('IF-B', 'Informatik', 'fk07')`)
	for _, ancode := range ancodes {
		if _, err := db.pool.Exec(ctx, `
			insert into exam (semester_id, ancode, source, module, main_examer,
			                  main_examer_id, exam_type, exam_type_full, duration_min)
			values ('2026-WS', $1, 'zpa', 'Modul', 'Braun', 1, 'schriftlich', 'schriftliche Prüfung', 90)`,
			ancode); err != nil {
			t.Fatalf("seed exam %d: %v", ancode, err)
		}
	}
}

func exec(t *testing.T, db *PG, sql string, args ...any) {
	t.Helper()
	if _, err := db.pool.Exec(t.Context(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func count(t *testing.T, db *PG, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := db.pool.QueryRow(t.Context(), sql, args...).Scan(&n); err != nil {
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
	db := newTestPG(t)
	ctx := t.Context()

	// Before: three exams, and the planner has done real work on 300.
	seedExamFixtures(t, db, 100, 200, 300)
	exec(t, db, `insert into exam_constraint (semester_id, ancode, online, exclude_days)
	             values ('2026-WS', 300, true, array['2027-01-15 00:00+01'::timestamptz])`)
	exec(t, db, `insert into exam_room_constraint (semester_id, ancode, exahm) values ('2026-WS', 300, true)`)
	exec(t, db, `insert into exam_duration_override (semester_id, ancode, duration_min) values ('2026-WS', 300, 120)`)
	exec(t, db, `insert into exam_to_plan (semester_id, ancode, to_plan) values ('2026-WS', 300, true)`)

	// The re-import: 100 and 200 come again (upsert, module changed on 100),
	// 400 and 500 are new, 300 is gone from ZPA.
	exec(t, db, `
		insert into exam (semester_id, ancode, source, module, main_examer, main_examer_id,
		                  exam_type, exam_type_full, duration_min)
		values ('2026-WS', 100, 'zpa', 'Analysis (neu)', 'Braun', 1, 'schriftlich', 'schriftliche Prüfung', 90),
		       ('2026-WS', 200, 'zpa', 'Modul',          'Braun', 1, 'schriftlich', 'schriftliche Prüfung', 90),
		       ('2026-WS', 400, 'zpa', 'Neu A',          'Braun', 1, 'schriftlich', 'schriftliche Prüfung', 90),
		       ('2026-WS', 500, 'zpa', 'Neu B',          'Braun', 1, 'schriftlich', 'schriftliche Prüfung', 90)
		on conflict (semester_id, ancode) do update set
		    module = excluded.module, duration_min = excluded.duration_min,
		    withdrawn_at = null`)
	exec(t, db, `
		update exam set withdrawn_at = now()
		where semester_id = '2026-WS' and source = 'zpa'
		  and ancode <> all($1::int[])`, []int{100, 200, 400, 500})

	// The two new ones arrived, the update took, nothing was lost.
	if n := count(t, db, `select count(*) from exam where semester_id='2026-WS'`); n != 5 {
		t.Errorf("exam count = %d, want 5", n)
	}
	if n := count(t, db, `select count(*) from exam
	                      where semester_id='2026-WS' and withdrawn_at is null`); n != 4 {
		t.Errorf("active exam count = %d, want 4", n)
	}
	var module string
	if err := db.pool.QueryRow(ctx,
		`select module from exam where semester_id='2026-WS' and ancode=100`).Scan(&module); err != nil {
		t.Fatalf("read module: %v", err)
	}
	if module != "Analysis (neu)" {
		t.Errorf("module = %q, want the upserted value", module)
	}

	// The withdrawn exam is marked, not gone ...
	if n := count(t, db, `select count(*) from exam
	                      where semester_id='2026-WS' and ancode=300 and withdrawn_at is not null`); n != 1 {
		t.Error("exam 300 should be marked withdrawn, not deleted")
	}

	// ... and every piece of hand-entered work on it survived.
	for _, table := range []string{
		"exam_constraint", "exam_room_constraint", "exam_duration_override", "exam_to_plan",
	} {
		if n := count(t, db,
			`select count(*) from `+table+` where semester_id='2026-WS' and ancode=300`); n != 1 {
			t.Errorf("%s for the withdrawn exam is gone -- the import destroyed planner input", table)
		}
	}
}

// TestDeletingAnExamCascades is the other half of the contract, and the reason the
// import must not delete: a real DELETE does take the overlay with it. That is
// correct for a deliberate act (dropping a semester, removing an external exam the
// planner created) and catastrophic for an import.
func TestDeletingAnExamCascades(t *testing.T) {
	db := newTestPG(t)

	seedExamFixtures(t, db, 300)
	exec(t, db, `insert into exam_constraint (semester_id, ancode, online) values ('2026-WS', 300, true)`)

	exec(t, db, `delete from exam where semester_id='2026-WS' and ancode=300`)

	if n := count(t, db, `select count(*) from exam_constraint where semester_id='2026-WS'`); n != 0 {
		t.Errorf("constraint count = %d, want 0 -- the cascade did not fire", n)
	}
}

// TestSemesterDeletionCascades: dropping a workspace must take its whole contents
// with it, since nothing outside it can reference them.
func TestSemesterDeletionCascades(t *testing.T) {
	db := newTestPG(t)

	seedExamFixtures(t, db, 100)
	exec(t, db, `insert into exam_constraint (semester_id, ancode, online) values ('2026-WS', 100, true)`)
	exec(t, db, `insert into semester_config_input (semester_id, config) values ('2026-WS', '{}')`)

	exec(t, db, `delete from semester where id='2026-WS'`)

	for _, table := range []string{"exam", "exam_constraint", "semester_config_input"} {
		if n := count(t, db, `select count(*) from `+table); n != 0 {
			t.Errorf("%s still has %d rows after the semester was deleted", table, n)
		}
	}
}
