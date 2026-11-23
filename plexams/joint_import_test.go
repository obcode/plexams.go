package plexams

import (
	"context"
	"strings"
	"testing"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
	"github.com/obcode/plexams.go/plexams/planstate"
)

const jointCSVHeader = "Nr;Modulname;Prüfungsform;Dauer;Erstpruefender;IstWiederholung;Studiengruppe;Prüfungsplanung\n"

// jointImportPlexams returns a Plexams on a fresh database with the joint study
// programs DE and ID, after an initial import of one FK03 exam per program.
func jointImportPlexams(t *testing.T) *Plexams {
	t.Helper()
	ctx := t.Context()
	pg := pgtest.NewDBWithSemester(t)
	p := &Plexams{dbClient: pg, semester: "2026-WS"}
	p.planState = planstate.New(pg, planningPhaseDefs, p.planningConditions())

	faculty := "MUC.DAI"
	for _, prog := range []string{"DE", "ID"} {
		if err := pg.UpsertStudyProgram(ctx, &model.StudyProgram{
			Shortname: prog, Name: prog, Category: "joint", JointFaculty: &faculty,
		}); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := p.ImportJointExams(ctx, jointCSVHeader+
		"12;Mathe;schriftlich;90;Prof A;;DE;FK03\n"+
		"56;Info;schriftlich;60;Prof C;;ID;FK12\n"); err != nil {
		t.Fatalf("initial import: %v", err)
	}
	return p
}

// jointState is what an import changes: the imported exams per program and the
// external exams generated from them.
func jointState(t *testing.T, p *Plexams) (de, id, external int) {
	t.Helper()
	ctx := context.Background()
	deExams, err := p.dbClient.JointExamsForProgram(ctx, "DE")
	if err != nil {
		t.Fatal(err)
	}
	idExams, err := p.dbClient.JointExamsForProgram(ctx, "ID")
	if err != nil {
		t.Fatal(err)
	}
	ext, err := p.dbClient.ExternalExams(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return len(deExams), len(idExams), len(ext)
}

// A program that is not in the study program table used to fail with the raw
// foreign-key violation joint_exam_program_fkey -- after the programs sorted before
// it had already been replaced. It is refused up front now, naming the program.
func TestImportJointExamsRefusesAnUnknownProgram(t *testing.T) {
	p := jointImportPlexams(t)

	_, err := p.ImportJointExams(t.Context(), jointCSVHeader+
		"12;Mathe;schriftlich;90;Prof A;;DE;FK03\n"+
		"13;Physik;schriftlich;90;Prof B;;DE;FK03\n"+
		"77;Chemie;schriftlich;90;Prof D;;XY;FK09\n"+
		"78;Biologie;schriftlich;90;Prof E;;AB;FK09\n")
	if err == nil {
		t.Fatal("import with unknown programs succeeded, want an error")
	}
	for _, want := range []string{"AB, XY", "nichts importiert"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "fkey") || strings.Contains(err.Error(), "SQLSTATE") {
		t.Errorf("error %q is the raw database error", err)
	}

	if de, id, ext := jointState(t, p); de != 1 || id != 1 || ext != 2 {
		t.Errorf("after refused import: DE=%d ID=%d external=%d, want 1/1/2 (unchanged)", de, id, ext)
	}
}

// The import is one transaction: when a later program fails, the programs before
// it are not left replaced (and their external exams not regenerated).
func TestImportJointExamsIsAtomic(t *testing.T) {
	p := jointImportPlexams(t)

	// DE sorts first and would succeed; ID carries the same Nr twice, which the
	// primary key of joint_exam rejects.
	_, err := p.ImportJointExams(t.Context(), jointCSVHeader+
		"12;Mathe;schriftlich;90;Prof A;;DE;FK03\n"+
		"13;Physik;schriftlich;90;Prof B;;DE;FK03\n"+
		"56;Info;schriftlich;60;Prof C;;ID;FK12\n"+
		"56;Info;schriftlich;60;Prof C;;ID;FK12\n")
	if err == nil {
		t.Fatal("import with a duplicate exam succeeded, want an error")
	}

	if de, id, ext := jointState(t, p); de != 1 || id != 1 || ext != 2 {
		t.Errorf("after failed import: DE=%d ID=%d external=%d, want 1/1/2 (rolled back)", de, id, ext)
	}
}

// A clean re-import still goes through: the transaction must not change the result.
func TestImportJointExamsReplacesAndGenerates(t *testing.T) {
	p := jointImportPlexams(t)

	res, err := p.ImportJointExams(t.Context(), jointCSVHeader+
		"12;Mathe;schriftlich;90;Prof A;;DE;FK03\n"+
		"13;Physik;schriftlich;90;Prof B;;DE;FK03\n"+
		"14;Statistik;schriftlich;90;Prof F;;DE;FK07\n")
	if err != nil {
		t.Fatalf("ImportJointExams: %v", err)
	}
	if res.ExamsImported != 3 || res.ExamsCreated != 1 || res.ExamsExisting != 1 || res.ExamsSkippedFk07 != 1 {
		t.Errorf("result = %+v, want imported 3, created 1, existing 1, skipped FK07 1", res)
	}
	if de, id, ext := jointState(t, p); de != 3 || id != 1 || ext != 3 {
		t.Errorf("after import: DE=%d ID=%d external=%d, want 3/1/3", de, id, ext)
	}
}
