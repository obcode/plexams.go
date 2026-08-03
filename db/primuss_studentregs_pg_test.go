package db_test

import (
	"testing"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/internal/pgtest"
)

func seedStudentReg(t *testing.T, pg *db.PG, program, studentProgram string, ancode int, mtknr, name string) {
	t.Helper()
	exec(t, pg, `insert into studentreg (semester_id, program, student_program, primuss_ancode, mtknr, name, group_name, presence)
	             values ('2026-WS', $1, $2, $3, $4, $5, '2', 'p')`,
		program, studentProgram, ancode, mtknr, name)
}

// TestStudentRegProgramIsTheStudentsOwn is the Go-side half of the two-programs
// finding. model.StudentReg.Program feeds the NTA programme check,
// model.Student.Program and the FK07 statistic -- all three want the student's
// own programme, not the one whose exam they registered for.
func TestStudentRegProgramIsTheStudentsOwn(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")
	seedPrimussExam(t, pg, "IF-B", 100, "Analysis")
	seedStudentReg(t, pg, "IF-B", "EI", 100, "00012345", "Zweitfach, Eine")

	regs, err := pg.GetPrimussStudentRegsForProgrammAncode(ctx, "IF-B", 100)
	if err != nil {
		t.Fatalf("GetPrimussStudentRegsForProgrammAncode: %v", err)
	}
	if len(regs) != 1 {
		t.Fatalf("len = %d, want 1", len(regs))
	}
	if regs[0].Program != "EI" {
		t.Errorf("Program = %q, want the student's own EI -- not the exam's IF-B", regs[0].Program)
	}
	if regs[0].Mtknr != "00012345" {
		t.Errorf("Mtknr = %q, want %q", regs[0].Mtknr, "00012345")
	}
	if regs[0].PrimussAncode != 100 {
		t.Errorf("PrimussAncode = %d, want 100", regs[0].PrimussAncode)
	}
	if regs[0].Group != "2" {
		t.Errorf("Group = %q, want %q", regs[0].Group, "2")
	}
	if regs[0].Presence != "p" {
		t.Errorf("Presence = %q, want %q", regs[0].Presence, "p")
	}
}

func TestStudentRegsSortedByName(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")
	seedPrimussExam(t, pg, "IF-B", 100, "Analysis")
	seedStudentReg(t, pg, "IF-B", "IF-B", 100, "3", "Zuletzt, Zacharias")
	seedStudentReg(t, pg, "IF-B", "IF-B", 100, "1", "Anfang, Anna")
	seedStudentReg(t, pg, "IF-B", "IF-B", 100, "2", "Mitte, Martin")

	regs, err := pg.GetPrimussStudentRegsForProgrammAncode(ctx, "IF-B", 100)
	if err != nil {
		t.Fatalf("GetPrimussStudentRegsForProgrammAncode: %v", err)
	}
	want := []string{"Anfang, Anna", "Mitte, Martin", "Zuletzt, Zacharias"}
	if len(regs) != len(want) {
		t.Fatalf("len = %d, want %d", len(regs), len(want))
	}
	for i, w := range want {
		if regs[i].Name != w {
			t.Errorf("[%d].Name = %q, want %q", i, regs[i].Name, w)
		}
	}
}

func TestStudentRegsGroupedPerAncodeAndPerStudent(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")
	seedPrimussExam(t, pg, "IF-B", 100, "Analysis")
	seedPrimussExam(t, pg, "IF-B", 200, "Lineare Algebra")
	seedStudentReg(t, pg, "IF-B", "IF-B", 100, "00000001", "Anfang, Anna")
	seedStudentReg(t, pg, "IF-B", "IF-B", 200, "00000001", "Anfang, Anna")
	seedStudentReg(t, pg, "IF-B", "EI", 100, "00000002", "Mitte, Martin")

	perAncode, err := pg.GetPrimussStudentRegsPerAncode(ctx, "IF-B")
	if err != nil {
		t.Fatalf("GetPrimussStudentRegsPerAncode: %v", err)
	}
	if len(perAncode[100]) != 2 || len(perAncode[200]) != 1 {
		t.Errorf("per ancode = %d/%d, want 2/1", len(perAncode[100]), len(perAncode[200]))
	}

	perStudent, err := pg.GetPrimussStudentRegsPerStudent(ctx, "IF-B")
	if err != nil {
		t.Fatalf("GetPrimussStudentRegsPerStudent: %v", err)
	}
	if len(perStudent["00000001"]) != 2 || len(perStudent["00000002"]) != 1 {
		t.Errorf("per student = %d/%d, want 2/1",
			len(perStudent["00000001"]), len(perStudent["00000002"]))
	}

	all, err := pg.StudentRegsForProgram(ctx, "IF-B")
	if err != nil {
		t.Fatalf("StudentRegsForProgram: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("len(StudentRegsForProgram) = %d, want 3", len(all))
	}
}

// The registrations have no key to primuss_exam, so the exam rename does not
// reach them -- ChangeAncodeInStudentRegs is what moves them.
func TestChangeAncodeInStudentRegs(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")
	seedPrimussExam(t, pg, "IF-B", 100, "Analysis")
	seedStudentReg(t, pg, "IF-B", "IF-B", 100, "00000001", "Anfang, Anna")
	seedStudentReg(t, pg, "IF-B", "IF-B", 100, "00000002", "Mitte, Martin")

	if _, err := pg.ChangeAncode(ctx, "IF-B", 100, 111); err != nil {
		t.Fatalf("ChangeAncode: %v", err)
	}
	regs, err := pg.ChangeAncodeInStudentRegs(ctx, "IF-B", 100, 111)
	if err != nil {
		t.Fatalf("ChangeAncodeInStudentRegs: %v", err)
	}
	if len(regs) != 2 {
		t.Fatalf("len = %d, want 2", len(regs))
	}
	for _, reg := range regs {
		if reg.PrimussAncode != 111 {
			t.Errorf("PrimussAncode = %d, want 111", reg.PrimussAncode)
		}
	}
	if n := count(t, pg, `select count(*) from studentreg where primuss_ancode = 100`); n != 0 {
		t.Errorf("%d registrations still carry the old ancode", n)
	}
}

func TestDuplicateStudentRegsAreReported(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")
	seedPrimussExam(t, pg, "IF-B", 100, "Analysis")
	seedStudentReg(t, pg, "IF-B", "IF-B", 100, "00000001", "Doppelt, Dora")
	seedStudentReg(t, pg, "IF-B", "IF-B", 100, "00000001", "Doppelt, Dora")
	seedStudentReg(t, pg, "IF-B", "IF-B", 100, "00000002", "Einmal, Erna")

	duplicates, err := pg.DuplicateStudentRegs(ctx, "IF-B")
	if err != nil {
		t.Fatalf("DuplicateStudentRegs: %v", err)
	}
	if len(duplicates) != 1 {
		t.Fatalf("len = %d, want 1", len(duplicates))
	}
	if duplicates[0].Mtknr != "00000001" || duplicates[0].Count != 2 || duplicates[0].Ancode != 100 {
		t.Errorf("duplicate = %+v, want {IF-B 100 00000001 2}", duplicates[0])
	}
	if duplicates[0].Program != "IF-B" {
		t.Errorf("Program = %q, want IF-B", duplicates[0].Program)
	}
}

// TestRemoveStudentRegDeletesExactlyOne is where the duplicate case bites. The
// Mongo DeleteOne removed one document; a plain DELETE ... WHERE would take the
// legitimate registration along with the duplicate, and this method exists
// precisely to clean duplicates up.
func TestRemoveStudentRegDeletesExactlyOne(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")
	seedPrimussExam(t, pg, "IF-B", 100, "Analysis")
	exec(t, pg, `insert into primuss_count (semester_id, program, ancode, total)
	             values ('2026-WS', 'IF-B', 100, 2)`)
	seedStudentReg(t, pg, "IF-B", "IF-B", 100, "00000001", "Doppelt, Dora")
	seedStudentReg(t, pg, "IF-B", "IF-B", 100, "00000001", "Doppelt, Dora")

	deleted, err := pg.RemoveStudentReg(ctx, "IF-B", 100, "00000001")
	if err != nil {
		t.Fatalf("RemoveStudentReg: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}
	if n := count(t, pg, `select count(*) from studentreg where mtknr = '00000001'`); n != 1 {
		t.Errorf("%d registrations left, want 1 -- the delete took both copies", n)
	}
	// The counter followed, in the same transaction.
	sum, err := pg.GetStudentRegsCount(ctx, "IF-B", 100)
	if err != nil {
		t.Fatalf("GetStudentRegsCount: %v", err)
	}
	if sum != 1 {
		t.Errorf("count = %d, want 1", sum)
	}
}

func TestRemoveStudentRegThatIsNotThere(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")
	seedPrimussExam(t, pg, "IF-B", 100, "Analysis")
	exec(t, pg, `insert into primuss_count (semester_id, program, ancode, total)
	             values ('2026-WS', 'IF-B', 100, 5)`)

	deleted, err := pg.RemoveStudentReg(ctx, "IF-B", 100, "00009999")
	if err != nil {
		t.Fatalf("RemoveStudentReg: %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0", deleted)
	}
	// The counter must NOT move when nothing was deleted.
	sum, err := pg.GetStudentRegsCount(ctx, "IF-B", 100)
	if err != nil {
		t.Fatalf("GetStudentRegsCount: %v", err)
	}
	if sum != 5 {
		t.Errorf("count = %d, want 5 -- the counter moved although nothing was deleted", sum)
	}
}

// TestAddStudentRegKeepsCounterInStep is the drift cause the Mongo version could
// only avoid on a replica set: registration and counter are one transaction here,
// unconditionally.
func TestAddStudentRegAndCounterAreOneTransaction(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")
	seedPrimussExam(t, pg, "IF-B", 100, "Analysis")
	exec(t, pg, `insert into primuss_count (semester_id, program, ancode, total) values ('2026-WS', 'IF-B', 100, 0)`)
	seedPreparedStudent(t, pg, "00000001", "Neu, Nina")

	if err := pg.AddStudentReg(ctx, "IF-B", 100, "00000001"); err != nil {
		t.Fatalf("AddStudentReg: %v", err)
	}

	regs, err := pg.GetPrimussStudentRegsForProgrammAncode(ctx, "IF-B", 100)
	if err != nil {
		t.Fatalf("GetPrimussStudentRegsForProgrammAncode: %v", err)
	}
	if len(regs) != 1 {
		t.Fatalf("len = %d, want 1", len(regs))
	}
	if regs[0].Name != "Neu, Nina" {
		t.Errorf("Name = %q, want %q -- it comes from the prepared student", regs[0].Name, "Neu, Nina")
	}
	// The manual add does not know the student's own programme, exactly like the
	// Mongo insert, which wrote AnCode, MTKNR and name and nothing else.
	if regs[0].Program != "" {
		t.Errorf("Program = %q, want the empty string", regs[0].Program)
	}

	sum, err := pg.GetStudentRegsCount(ctx, "IF-B", 100)
	if err != nil {
		t.Fatalf("GetStudentRegsCount: %v", err)
	}
	if sum != 1 {
		t.Errorf("count = %d, want 1", sum)
	}
}

// The whole point of the transaction: a failure in the second write must undo the
// first. The unknown student makes AddStudentReg fail before it inserts, so the
// counter must not have moved either.
func TestAddStudentRegRollsBackOnAnUnknownStudent(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")
	seedPrimussExam(t, pg, "IF-B", 100, "Analysis")
	exec(t, pg, `insert into primuss_count (semester_id, program, ancode, total) values ('2026-WS', 'IF-B', 100, 3)`)

	if err := pg.AddStudentReg(ctx, "IF-B", 100, "00009999"); err == nil {
		t.Fatal("AddStudentReg accepted a student who has not been prepared")
	}

	if n := count(t, pg, `select count(*) from studentreg`); n != 0 {
		t.Errorf("%d registrations were written although the add failed", n)
	}
	sum, err := pg.GetStudentRegsCount(ctx, "IF-B", 100)
	if err != nil {
		t.Fatalf("GetStudentRegsCount: %v", err)
	}
	if sum != 3 {
		t.Errorf("count = %d, want 3 -- the counter moved although the add failed", sum)
	}
}

func TestStudentRegsCountMismatches(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")
	for _, ancode := range []int{100, 200, 300, 400} {
		seedPrimussExam(t, pg, "IF-B", ancode, "Modul")
	}
	// 100: consistent. 200: counter disagrees. 300: no counter at all.
	// 400: a counter but no registrations -- not reported, as before.
	exec(t, pg, `insert into primuss_count (semester_id, program, ancode, total)
	             values ('2026-WS', 'IF-B', 100, 1), ('2026-WS', 'IF-B', 200, 5), ('2026-WS', 'IF-B', 400, 9)`)
	seedStudentReg(t, pg, "IF-B", "IF-B", 100, "00000001", "Eine, Person")
	seedStudentReg(t, pg, "IF-B", "IF-B", 200, "00000001", "Eine, Person")
	seedStudentReg(t, pg, "IF-B", "IF-B", 300, "00000001", "Eine, Person")

	mismatches, err := pg.StudentRegsCountMismatches(ctx, "IF-B")
	if err != nil {
		t.Fatalf("StudentRegsCountMismatches: %v", err)
	}
	if len(mismatches) != 2 {
		t.Fatalf("len = %d, want 2 -- got %+v", len(mismatches), mismatches)
	}
	if mismatches[0].Ancode != 200 || mismatches[0].Stored != 1 || mismatches[0].Recorded != 5 {
		t.Errorf("mismatches[0] = %+v, want ancode 200, stored 1, recorded 5", mismatches[0])
	}
	if mismatches[1].Ancode != 300 || mismatches[1].Stored != 1 || mismatches[1].Recorded != db.NoCountDocument {
		t.Errorf("mismatches[1] = %+v, want ancode 300, stored 1, recorded NoCountDocument", mismatches[1])
	}
}

func TestStudentRegsCountMismatchesStaySilentWhenConsistent(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")
	seedPrimussExam(t, pg, "IF-B", 100, "Analysis")
	exec(t, pg, `insert into primuss_count (semester_id, program, ancode, total) values ('2026-WS', 'IF-B', 100, 2)`)
	seedStudentReg(t, pg, "IF-B", "IF-B", 100, "00000001", "Eine, Person")
	seedStudentReg(t, pg, "IF-B", "IF-B", 100, "00000002", "Andere, Person")

	mismatches, err := pg.StudentRegsCountMismatches(ctx, "IF-B")
	if err != nil {
		t.Fatalf("StudentRegsCountMismatches: %v", err)
	}
	if len(mismatches) != 0 {
		t.Errorf("mismatches = %+v, want none", mismatches)
	}
}
