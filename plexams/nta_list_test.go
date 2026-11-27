package plexams

import (
	"bytes"
	"testing"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
	"github.com/xuri/excelize/v2"
)

// ntaListXLSX builds an NTA list in the layout of the PRIMUSS export. All data
// is invented.
func ntaListXLSX(t *testing.T, fk string, rows [][]string) []byte {
	t.Helper()
	f := excelize.NewFile()
	all := [][]string{
		{"Hochschule München", "", "", "", "", "", "", "", "27.11.2026 10:05:29"},
		{"Nachteilsausgleich (1)"},
		{"STG-Abs: *"},
		{"FK " + fk},
		{"für Semester 226"},
		{"fk", "stg", "mtknr", "vname", "name", "beginn_studium", "nachteilsausgleich", "semester", "anmeldungen_aktsemester"},
	}
	all = append(all, rows...)
	for i, r := range all {
		for j, v := range r {
			cell, _ := excelize.CoordinatesToCellName(j+1, i+1)
			if err := f.SetCellStr("Sheet1", cell, v); err != nil {
				t.Fatal(err)
			}
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func strPtr(s string) *string { return &s }

func TestNTAListUploadPreviewApply(t *testing.T) {
	dbClient := pgtest.NewDBWithSemester(t)
	ctx := t.Context()
	p := &Plexams{dbClient: dbClient, semester: "2026-WS"}

	for _, sp := range []*model.StudyProgram{
		{Shortname: "IF", Name: "Informatik", Degree: strPtr("Bachelor"), Category: "fk07", Active: true},
		{Shortname: "DE", Name: "Data Engineering", Degree: strPtr("Bachelor"), Category: "joint", Active: true, JointFaculty: strPtr("MUC.DAI")},
	} {
		if err := dbClient.UpsertStudyProgram(ctx, sp); err != nil {
			t.Fatal(err)
		}
	}
	// an existing NTA with a different extension and one that matches exactly
	if _, err := dbClient.AddNta(ctx, &model.NTA{Name: "Muster, Erika", Mtknr: "00000001", Compensation: "10% Schreibzeitverlängerung",
		DeltaDurationPercent: 10, Program: "IF", From: "01.03.2026", Until: "Ende Studium"}); err != nil {
		t.Fatal(err)
	}
	if _, err := dbClient.AddNta(ctx, &model.NTA{Name: "Beispiel, Max", Mtknr: "00000002", Compensation: "20% Schreibzeitverlängerung",
		DeltaDurationPercent: 20, Program: "IF", From: "01.03.2026", Until: "Ende Studium"}); err != nil {
		t.Fatal(err)
	}
	if _, err := dbClient.ReplacePrimussStudentRegs(ctx, "IF", []db.PrimussStudentRegRow{
		{Mtknr: "00000001", PrimussAncode: 101, StudentProgram: "IF"},
		{Mtknr: "00000001", PrimussAncode: 103, StudentProgram: "IF"},
	}); err != nil {
		t.Fatal(err)
	}

	fk07 := ntaListXLSX(t, "07", [][]string{
		{"7", "IF-B", "00000001", "Erika", "Muster", "225", "25% Schreibzeitverlängerung, gesonderter Raum", "*", "IF-B:101 | IF-B:102"},
		{"7", "IF-B", "00000002", "Max", "Beispiel", "224", "20% Schreibzeitverlängerung", "*", ""},
		{"7", "IF-B", "00000003", "Anna", "Neu", "226", "10% Schreibzeitverlängerung", "226:127", ""},
		{"7", "IF-B", "00000004", "Kim", "Unklar", "226", "Sonderregelung", "*", ""},
	})
	fk21 := ntaListXLSX(t, "21", [][]string{
		{"21", "DE-B", "00000005", "Lou", "Daten", "226", "10% Schreibzeitverlängerung", "*", "DE-B:201"},
	})

	sums, err := p.UploadNTALists(ctx, []NTAListFile{
		{Filename: "226_07_Liste_NTA_Stand_27-11-26.xlsx", Data: fk07},
		{Filename: "226_21_Liste_NTA_Stand_27-11-26.xlsx", Data: fk21},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sums) != 2 || !sums[0].FirstUpload || sums[0].NewRows != 4 {
		t.Fatalf("summaries = %+v", sums[0])
	}

	preview, err := p.NTAListPreview(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Lists) != 2 || !preview.PrimussRegsAvailable {
		t.Fatalf("lists = %d, regs = %v", len(preview.Lists), preview.PrimussRegsAvailable)
	}
	if preview.Lists[0].Faculty != "FK 07" || preview.Lists[1].Faculty != "FK 21 (MUC.DAI)" || !preview.Lists[0].SemesterMatches {
		t.Errorf("faculties = %q, %q", preview.Lists[0].Faculty, preview.Lists[1].Faculty)
	}
	rows := preview.Lists[0].Rows
	want := []model.NTAListRowStatus{model.NTAListRowStatusChanged, model.NTAListRowStatusUnchanged,
		model.NTAListRowStatusNew, model.NTAListRowStatusProblem}
	for i, w := range want {
		if rows[i].Status != w {
			t.Errorf("row %d: status %s, want %s (problems %v)", i, rows[i].Status, w, rows[i].Problems)
		}
	}
	changed := map[string]bool{}
	for _, c := range rows[0].Changes {
		changed[c.Field] = c.Significant
	}
	if !changed["deltaDurationPercent"] || !changed["needsRoomAlone"] || changed["compensation"] || len(changed) != 3 {
		t.Errorf("changes = %+v", changed)
	}
	if rows[0].Proposal.From != "01.03.2026" {
		t.Errorf("existing from must be kept, got %q", rows[0].Proposal.From)
	}
	if len(rows[0].RegsOnlyInList) != 1 || rows[0].RegsOnlyInList[0].PrimussAncode != 102 ||
		len(rows[0].RegsOnlyInPrimuss) != 1 || rows[0].RegsOnlyInPrimuss[0].PrimussAncode != 103 {
		t.Errorf("regs: only in list %+v, only in primuss %+v", rows[0].RegsOnlyInList, rows[0].RegsOnlyInPrimuss)
	}
	n := rows[2].Proposal
	if n.From != "27.11.2026" || n.Until != "2027-SS" || n.Program != "IF" || n.Name != "Neu, Anna" || n.DeltaDurationPercent != 10 {
		t.Errorf("new proposal = %+v", n)
	}
	if p := preview.Lists[1].Rows[0].Proposal; p.Program != "DE" {
		t.Errorf("MUC.DAI program = %q", p.Program)
	}

	toInput := func(pr *model.NTAListProposal) *model.NTAInput {
		return &model.NTAInput{Name: pr.Name, Email: pr.Email, Mtknr: pr.Mtknr, Compensation: pr.Compensation,
			DeltaDurationPercent: pr.DeltaDurationPercent, NeedsRoomAlone: pr.NeedsRoomAlone, NeedsHardware: pr.NeedsHardware,
			Program: pr.Program, From: pr.From, Until: pr.Until}
	}
	res, err := p.ApplyNTAList(ctx, []*model.NTAListApplyItem{
		{Action: model.NTAListApplyActionUpdate, Nta: toInput(rows[0].Proposal)},
		{Action: model.NTAListApplyActionAdd, Nta: toInput(rows[2].Proposal)},
	})
	if err != nil || res.Added != 1 || res.Updated != 1 {
		t.Fatalf("apply = %+v, %v", res, err)
	}

	// all or nothing: adding an existing NTA fails and rolls back the other item
	if _, err := p.ApplyNTAList(ctx, []*model.NTAListApplyItem{
		{Action: model.NTAListApplyActionAdd, Nta: toInput(preview.Lists[1].Rows[0].Proposal)},
		{Action: model.NTAListApplyActionAdd, Nta: toInput(rows[2].Proposal)},
	}); err == nil {
		t.Fatal("adding an existing NTA must fail")
	}
	if nta, _ := dbClient.Nta(ctx, "00000005"); nta != nil {
		t.Error("failed apply was not rolled back")
	}

	preview, err = p.NTAListPreview(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if st := preview.Lists[0].Rows[i].Status; st != model.NTAListRowStatusUnchanged {
			t.Errorf("after apply row %d: %s %+v", i, st, preview.Lists[0].Rows[i].Changes)
		}
	}

	// uploading again: one row changed, one removed, first_seen survives
	firstSeen := preview.Lists[0].Rows[0].FirstSeen
	fk07b := ntaListXLSX(t, "07", [][]string{
		{"7", "IF-B", "00000001", "Erika", "Muster", "225", "25% Schreibzeitverlängerung, gesonderter Raum", "*", "IF-B:101 | IF-B:102"},
		{"7", "IF-B", "00000002", "Max", "Beispiel", "224", "30% Schreibzeitverlängerung", "*", ""},
		{"7", "IF-B", "00000003", "Anna", "Neu", "226", "10% Schreibzeitverlängerung", "226:127", ""},
	})
	sums, err = p.UploadNTALists(ctx, []NTAListFile{{Filename: "226_07_Liste_NTA_Stand_28-11-26.xlsx", Data: fk07b}})
	if err != nil {
		t.Fatal(err)
	}
	s := sums[0]
	if s.FirstUpload || s.NewRows != 0 || s.ChangedRows != 1 || s.RemovedRows != 1 || s.UnchangedRows != 2 {
		t.Errorf("second upload summary = %+v", s)
	}
	preview, _ = p.NTAListPreview(ctx)
	if !preview.Lists[0].Rows[0].FirstSeen.Equal(firstSeen) {
		t.Errorf("first seen changed: %v -> %v", firstSeen, preview.Lists[0].Rows[0].FirstSeen)
	}

	if ok, err := p.RemoveNTAList(ctx, "21"); !ok || err != nil {
		t.Errorf("remove = %v, %v", ok, err)
	}
	preview, _ = p.NTAListPreview(ctx)
	if len(preview.Lists) != 1 {
		t.Errorf("lists after remove = %d", len(preview.Lists))
	}
}
