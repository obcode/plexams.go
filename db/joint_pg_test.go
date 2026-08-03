package db_test

import (
	"testing"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/internal/pgtest"
)

func testJointExam(program string, ancode int, module string) *db.JointExam {
	return &db.JointExam{
		PrimussAncode:  ancode,
		Module:         module,
		ExamType:       "schrP",
		Grading:        "benotet",
		Duration:       90,
		MainExamer:     "Dietrich, Benedikt",
		SecondExamer:   "Hobelsberger, Martin",
		IsRepeaterExam: "x",
		Program:        program,
		Planer:         "FK07",
	}
}

func TestJointExamRoundTrip(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "ID-B")

	want := testJointExam("ID-B", 101, "Computational Thinking")
	if err := pg.ReplaceJointExamsForProgram(ctx, "ID-B", []*db.JointExam{want}); err != nil {
		t.Fatalf("ReplaceJointExamsForProgram: %v", err)
	}

	got, err := pg.JointExam(ctx, "ID-B", 101)
	if err != nil {
		t.Fatalf("JointExam: %v", err)
	}
	for _, f := range []struct {
		name      string
		want, got string
	}{
		{"Module", want.Module, got.Module},
		{"ExamType", want.ExamType, got.ExamType},
		{"Grading", want.Grading, got.Grading},
		{"MainExamer", want.MainExamer, got.MainExamer},
		{"SecondExamer", want.SecondExamer, got.SecondExamer},
		{"IsRepeaterExam", want.IsRepeaterExam, got.IsRepeaterExam},
		{"Program", want.Program, got.Program},
		{"Planer", want.Planer, got.Planer},
	} {
		if f.want != f.got {
			t.Errorf("%s = %q, want %q", f.name, f.got, f.want)
		}
	}
	if got.PrimussAncode != want.PrimussAncode {
		t.Errorf("PrimussAncode = %d, want %d", got.PrimussAncode, want.PrimussAncode)
	}
	if got.Duration != want.Duration {
		t.Errorf("Duration = %d, want %d", got.Duration, want.Duration)
	}
}

func TestJointExamsReplaceAndSort(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "ID-B", "DE-B")

	if err := pg.ReplaceJointExamsForProgram(ctx, "ID-B", []*db.JointExam{
		testJointExam("ID-B", 111, "Drittes"),
		testJointExam("ID-B", 101, "Erstes"),
		testJointExam("ID-B", 102, "Zweites"),
	}); err != nil {
		t.Fatalf("ReplaceJointExamsForProgram: %v", err)
	}
	// The other program's exams must be untouched by a refill.
	if err := pg.ReplaceJointExamsForProgram(ctx, "DE-B", []*db.JointExam{
		testJointExam("DE-B", 101, "Anderes Programm"),
	}); err != nil {
		t.Fatalf("ReplaceJointExamsForProgram(DE-B): %v", err)
	}

	exams, err := pg.JointExamsForProgram(ctx, "ID-B")
	if err != nil {
		t.Fatalf("JointExamsForProgram: %v", err)
	}
	want := []int{101, 102, 111}
	if len(exams) != len(want) {
		t.Fatalf("len = %d, want %d", len(exams), len(want))
	}
	for i, w := range want {
		if exams[i].PrimussAncode != w {
			t.Errorf("[%d].PrimussAncode = %d, want %d", i, exams[i].PrimussAncode, w)
		}
	}

	// Refilling replaces rather than appends ...
	if err := pg.ReplaceJointExamsForProgram(ctx, "ID-B", []*db.JointExam{
		testJointExam("ID-B", 101, "Erstes"),
	}); err != nil {
		t.Fatalf("ReplaceJointExamsForProgram (second): %v", err)
	}
	exams, err = pg.JointExamsForProgram(ctx, "ID-B")
	if err != nil {
		t.Fatalf("JointExamsForProgram: %v", err)
	}
	if len(exams) != 1 {
		t.Errorf("len = %d, want 1 -- the refill appended", len(exams))
	}
	// ... and only for the program that was refilled.
	other, err := pg.JointExamsForProgram(ctx, "DE-B")
	if err != nil {
		t.Fatalf("JointExamsForProgram(DE-B): %v", err)
	}
	if len(other) != 1 {
		t.Errorf("the other program has %d exams, want 1 -- the refill crossed programs", len(other))
	}
}

func TestJointExamsEmptyIsNotNil(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "ID-B")
	// Replacing with nothing empties the program, which the CSV import does when
	// a program delivers no exams.
	if err := pg.ReplaceJointExamsForProgram(ctx, "ID-B", nil); err != nil {
		t.Fatalf("ReplaceJointExamsForProgram: %v", err)
	}

	exams, err := pg.JointExamsForProgram(ctx, "ID-B")
	if err != nil {
		t.Fatalf("JointExamsForProgram: %v", err)
	}
	if exams == nil {
		t.Fatal("JointExamsForProgram returned nil, want an empty slice")
	}
}

func TestJointLinkRoundTrip(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "DE-B")

	ancode := 539
	want := &db.JointLink{
		Program:       "DE-B",
		PrimussAncode: 101,
		Kind:          "zpa",
		Ancode:        &ancode,
		Status:        "linked",
		Source:        "auto",
		Module:        "Computational Thinking",
		MainExamer:    "Dietrich, Benedikt",
	}
	if err := pg.UpsertJointLink(ctx, want); err != nil {
		t.Fatalf("UpsertJointLink: %v", err)
	}

	got, err := pg.JointLink(ctx, "DE-B", 101)
	if err != nil {
		t.Fatalf("JointLink: %v", err)
	}
	if got == nil {
		t.Fatal("link is nil")
	}
	if got.Kind != "zpa" || got.Status != "linked" || got.Source != "auto" {
		t.Errorf("kind/status/source = %s/%s/%s", got.Kind, got.Status, got.Source)
	}
	if got.Ancode == nil || *got.Ancode != 539 {
		t.Errorf("Ancode = %v, want 539", got.Ancode)
	}
	if got.Module != want.Module || got.MainExamer != want.MainExamer {
		t.Errorf("snapshot = %q / %q", got.Module, got.MainExamer)
	}
}

// "Not linked yet" is a normal state, so an unresolved link carries no ancode.
func TestJointLinkUnresolvedHasNoAncode(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "DE-B")

	if err := pg.UpsertJointLink(ctx, &db.JointLink{
		Program:       "DE-B",
		PrimussAncode: 102,
		Kind:          "external",
		Status:        "unresolved",
		Source:        "auto",
		Module:        "Physik",
	}); err != nil {
		t.Fatalf("UpsertJointLink: %v", err)
	}

	got, err := pg.JointLink(ctx, "DE-B", 102)
	if err != nil {
		t.Fatalf("JointLink: %v", err)
	}
	if got.Ancode != nil {
		t.Errorf("Ancode = %v, want nil for an unresolved link", *got.Ancode)
	}
}

// TestJointLinkStatusAndAncodeAgree pins the check that retires the
// "linked with no ancode" line of ValidateDBReferences: it is now impossible to
// store rather than reported afterwards. Both directions are wrong.
func TestJointLinkStatusAndAncodeAgree(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "DE-B")

	err := pg.UpsertJointLink(ctx, &db.JointLink{
		Program: "DE-B", PrimussAncode: 103, Kind: "zpa",
		Status: "linked", Source: "auto",
	})
	if err == nil {
		t.Error("a link with status 'linked' and no ancode was accepted")
	}

	ancode := 539
	err = pg.UpsertJointLink(ctx, &db.JointLink{
		Program: "DE-B", PrimussAncode: 104, Kind: "zpa", Ancode: &ancode,
		Status: "unresolved", Source: "auto",
	})
	if err == nil {
		t.Error("a link with status 'unresolved' and an ancode was accepted")
	}
}

func TestJointLinkMissingReturnsNilNil(t *testing.T) {
	pg := pgtest.NewDB(t)

	seedPrimussFixtures(t, pg, "DE-B")

	got, err := pg.JointLink(t.Context(), "DE-B", 999)
	if err != nil {
		t.Fatalf("JointLink: %v", err)
	}
	if got != nil {
		t.Errorf("JointLink = %v, want nil", got)
	}
}

func TestJointLinksUpsertAndDelete(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "DE-B", "GS-B")

	links, err := pg.JointLinks(ctx)
	if err != nil {
		t.Fatalf("JointLinks: %v", err)
	}
	if links == nil {
		t.Fatal("JointLinks returned nil, want an empty slice")
	}

	ancode := 539
	for _, l := range []*db.JointLink{
		{Program: "GS-B", PrimussAncode: 102, Kind: "zpa", Ancode: &ancode, Status: "linked", Source: "auto"},
		{Program: "DE-B", PrimussAncode: 101, Kind: "zpa", Ancode: &ancode, Status: "linked", Source: "auto"},
	} {
		if err := pg.UpsertJointLink(ctx, l); err != nil {
			t.Fatalf("UpsertJointLink: %v", err)
		}
	}

	links, err = pg.JointLinks(ctx)
	if err != nil {
		t.Fatalf("JointLinks: %v", err)
	}
	if len(links) != 2 || links[0].Program != "DE-B" || links[1].Program != "GS-B" {
		t.Errorf("links = %+v, want DE-B before GS-B", links)
	}

	// Re-upserting the same key replaces rather than adds.
	if err := pg.UpsertJointLink(ctx, &db.JointLink{
		Program: "DE-B", PrimussAncode: 101, Kind: "external", Ancode: &ancode,
		Status: "linked", Source: "manual",
	}); err != nil {
		t.Fatalf("UpsertJointLink (second): %v", err)
	}
	got, err := pg.JointLink(ctx, "DE-B", 101)
	if err != nil {
		t.Fatalf("JointLink: %v", err)
	}
	if got.Kind != "external" || got.Source != "manual" {
		t.Errorf("kind/source = %s/%s, want external/manual", got.Kind, got.Source)
	}
	if n := count(t, pg, "select count(*) from joint_link"); n != 2 {
		t.Errorf("joint_link rows = %d, want 2", n)
	}

	if err := pg.DeleteJointLink(ctx, "DE-B", 101); err != nil {
		t.Fatalf("DeleteJointLink: %v", err)
	}
	if n := count(t, pg, "select count(*) from joint_link"); n != 1 {
		t.Errorf("joint_link rows = %d after the delete, want 1", n)
	}
	// Deleting one that is not there is not an error, as before.
	if err := pg.DeleteJointLink(ctx, "DE-B", 101); err != nil {
		t.Fatalf("DeleteJointLink (absent): %v", err)
	}
}

// A joint exam refill must not take the hand-curated links with it -- they are in
// their own table with no key to the exams, which is what makes the wholesale
// replace safe.
func TestJointExamRefillKeepsTheLinks(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "DE-B")
	if err := pg.ReplaceJointExamsForProgram(ctx, "DE-B", []*db.JointExam{
		testJointExam("DE-B", 101, "Computational Thinking"),
	}); err != nil {
		t.Fatalf("ReplaceJointExamsForProgram: %v", err)
	}
	ancode := 539
	if err := pg.UpsertJointLink(ctx, &db.JointLink{
		Program: "DE-B", PrimussAncode: 101, Kind: "zpa", Ancode: &ancode,
		Status: "linked", Source: "manual",
	}); err != nil {
		t.Fatalf("UpsertJointLink: %v", err)
	}

	if err := pg.ReplaceJointExamsForProgram(ctx, "DE-B", []*db.JointExam{
		testJointExam("DE-B", 101, "Computational Thinking (neu)"),
	}); err != nil {
		t.Fatalf("ReplaceJointExamsForProgram (second): %v", err)
	}

	link, err := pg.JointLink(ctx, "DE-B", 101)
	if err != nil {
		t.Fatalf("JointLink: %v", err)
	}
	if link == nil {
		t.Fatal("the re-import destroyed a hand-curated link")
	}
	if link.Source != "manual" {
		t.Errorf("Source = %q, want manual", link.Source)
	}
}
