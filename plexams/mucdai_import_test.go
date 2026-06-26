package plexams

import "testing"

func TestParseMucDaiCSVSemicolonAndUmlauts(t *testing.T) {
	csv := "\ufeffNr;Modulname;Prüfungsform;Dauer;Erstpruefender;IstWiederholung;Studiengruppe;Prüfungsplanung\n" +
		"12;Mathe;schriftlich;90;Prof A;;DE;FK07\n" +
		"34;Physik;schriftlich;120;Prof B;x;DE;FK03\n" +
		"56;Info;mündlich;30;Prof C;;ID;FK12\n"

	byProgram, err := parseMucDaiCSV(csv)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(byProgram["DE"]) != 2 {
		t.Errorf("DE: want 2, got %d", len(byProgram["DE"]))
	}
	if len(byProgram["ID"]) != 1 {
		t.Errorf("ID: want 1, got %d", len(byProgram["ID"]))
	}

	de0 := byProgram["DE"][0]
	if de0.PrimussAncode != 12 || de0.Module != "Mathe" || de0.Duration != 90 || de0.Planer != "FK07" {
		t.Errorf("DE[0] wrong: %+v", de0)
	}
	if byProgram["DE"][1].Planer != "FK03" {
		t.Errorf("DE[1] planer want FK03, got %s", byProgram["DE"][1].Planer)
	}
	if byProgram["ID"][0].ExamType != "mündlich" {
		t.Errorf("ID[0] examType want mündlich, got %s", byProgram["ID"][0].ExamType)
	}
}

func TestParseMucDaiCSVCommaAndMissingNr(t *testing.T) {
	csv := "Nr,Modulname,Studiengruppe,Prüfungsplanung\n" +
		"7,Test,GS,FK08\n" +
		",NoNr,GS,FK08\n" + // skipped (no Nr)
		"abc,BadNr,GS,FK08\n" // skipped (non-numeric)

	byProgram, err := parseMucDaiCSV(csv)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(byProgram["GS"]) != 1 {
		t.Fatalf("GS: want 1 valid row, got %d", len(byProgram["GS"]))
	}
	if byProgram["GS"][0].PrimussAncode != 7 {
		t.Errorf("want ancode 7, got %d", byProgram["GS"][0].PrimussAncode)
	}
}

func TestParseMucDaiCSVMissingRequiredColumn(t *testing.T) {
	if _, err := parseMucDaiCSV("Modulname,Studiengruppe\nMathe,DE\n"); err == nil {
		t.Error("expected error for missing Nr column")
	}
}
