package db

import (
	"slices"
	"testing"
)

// tablesWithoutSemesterID are the ones that legitimately have no semester_id
// column. Every other table must carry one.
//
// Note that a table HAVING semester_id does not make it semester-scoped:
// active_semester and scheduler_state are single-row global tables that merely
// point at a semester, and they are absent here for that reason -- they pass the
// check trivially.
//
// Adding a name here is a claim, and the reverse check below makes sure the claim
// is still true.
var tablesWithoutSemesterID = []string{
	// Master data: carries over between planning cycles.
	"nta",
	"room",
	"study_program",
	"permanent_non_invigilator",
	"app_user",
	"user_secret",
	"email_template",
	"planer",
	"anny_config",
	"generation_config",
	"aaspf_degree",

	// The registry itself -- its primary key IS the semester.
	"semester",

	// Scoped transitively: it hangs off a single planned_room row by surrogate id,
	// so it inherits that row's semester and cannot cross into another one. This is
	// the only such table; adding more of them dilutes the guarantee, because the
	// check below can no longer see the semester at all.
	"planned_room_student",

	// goose's own bookkeeping.
	"goose_db_version",
}

// TestEverySemesterScopedTableHasSemesterID is the cheap half of the "do not
// forget semester_id" guard.
//
// Under MongoDB the semester was the database, so it could not be forgotten -- the
// driver supplied it. Here it is a column, and a table without it is a table whose
// rows silently belong to every semester at once. The mistake is invisible until
// two semesters are in the database.
func TestEverySemesterScopedTableHasSemesterID(t *testing.T) {
	db := newTestPG(t)

	rows, err := db.pool.Query(t.Context(), `
		select t.tablename
		from pg_tables t
		where t.schemaname = 'public'
		  and not exists (
		      select 1 from information_schema.columns c
		      where c.table_schema = 'public'
		        and c.table_name = t.tablename
		        and c.column_name = 'semester_id')
		order by t.tablename`)
	if err != nil {
		t.Fatalf("query schema: %v", err)
	}
	defer rows.Close()

	var without []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		without = append(without, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}

	for _, name := range without {
		if !slices.Contains(tablesWithoutSemesterID, name) {
			t.Errorf("table %q has no semester_id column.\n"+
				"Either add one, or add it to tablesWithoutSemesterID and say why it may not have one.", name)
		}
	}

	// Keep the list honest in the other direction too: a global table that gained
	// a semester_id, or one that was dropped, must not linger here.
	for _, name := range tablesWithoutSemesterID {
		if name == "goose_db_version" {
			continue
		}
		if !slices.Contains(without, name) {
			t.Errorf("tablesWithoutSemesterID lists %q, but it has a semester_id now -- remove the entry", name)
		}
	}
}

// TestSemesterScopedForeignKeysCarrySemesterID checks the other half: a foreign
// key between two semester-scoped tables must be COMPOSITE. Referencing only an
// ancode would let a row in one semester point at a row in another, which is
// exactly the class of bug the composite keys exist to make unrepresentable.
//
// PostgreSQL already refuses `references exam(ancode)` on its own, because ancode
// alone is not unique there. What it cannot catch -- and what this test is for --
// is a reference through a SURROGATE key: planned_room.id is unique on its own, so
// a foreign key to it is accepted while carrying no semester at all.
//
// Verified to fail on a deliberately bad constraint before being trusted.
func TestSemesterScopedForeignKeysCarrySemesterID(t *testing.T) {
	db := newTestPG(t)

	rows, err := db.pool.Query(t.Context(), `
		select con.conname, src.relname, tgt.relname
		from pg_constraint con
		join pg_class src on src.oid = con.conrelid
		join pg_class tgt on tgt.oid = con.confrelid
		where con.contype = 'f'
		  -- both sides are semester-scoped ...
		  and exists (select 1 from information_schema.columns c
		              where c.table_name = src.relname and c.column_name = 'semester_id')
		  and exists (select 1 from information_schema.columns c
		              where c.table_name = tgt.relname and c.column_name = 'semester_id')
		  -- ... but semester_id is not among the referencing columns
		  and not exists (
		      select 1 from unnest(con.conkey) as k
		      join information_schema.columns c
		        on c.table_name = src.relname and c.ordinal_position = k
		      where c.column_name = 'semester_id')
		order by con.conname`)
	if err != nil {
		t.Fatalf("query constraints: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name, src, tgt string
		if err := rows.Scan(&name, &src, &tgt); err != nil {
			t.Fatalf("scan: %v", err)
		}
		t.Errorf("foreign key %s (%s -> %s) does not carry semester_id: "+
			"a row could reference another semester's row", name, src, tgt)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
}
