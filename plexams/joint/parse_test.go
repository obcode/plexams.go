package joint

import "testing"

func TestParseJointCSVSemicolonAndUmlauts(t *testing.T) {
	csv := "\ufeffNr;Modulname;Prüfungsform;Dauer;Erstpruefender;IstWiederholung;Studiengruppe;Prüfungsplanung\n" +
		"12;Mathe;schriftlich;90;Prof A;;DE;FK07\n" +
		"34;Physik;schriftlich;120;Prof B;x;DE;FK03\n" +
		"56;Info;mündlich;30;Prof C;;ID;FK12\n"

	byProgram, err := ParseCSV(csv)
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

// the real MUC.DAI format: tab-separated, mongoimport type suffixes in the header.
func TestParseJointCSVRealTabTypedHeader(t *testing.T) {
	csv := "Nr.int32()\tModulname.string()\tPrüfungsform.string()\tBewertung.string()\tDauer.int32()\tErstpruefender.string()\tZweitpruefender.string()\tIstWiederholung.string()\tStudiengruppe.string()\tPrüfungsplanung.string()\n" +
		"101\tComputational Thinking\tschrP\tbenotet\t90\tDietrich, Benedikt\tHobelsberger, Martin\tx\tDE\tFK07\n" +
		"112\tElektrotechnik\tschrP\tbenotet\t60\tPalme, Frank\tKüpper, Tilman\tx\tDE\tFK03\n" +
		"301\tStatistik und Stochastik\tschrP\tbenotet\t60\tShao, Shuai\tBrockhaus, Sarah\tx\tID\tFK07\n"

	byProgram, err := ParseCSV(csv)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(byProgram["DE"]) != 2 || len(byProgram["ID"]) != 1 {
		t.Fatalf("grouping wrong: DE=%d ID=%d", len(byProgram["DE"]), len(byProgram["ID"]))
	}
	de0 := byProgram["DE"][0]
	if de0.PrimussAncode != 101 || de0.Module != "Computational Thinking" || de0.Duration != 90 || de0.Planer != "FK07" {
		t.Errorf("DE[0] wrong: %+v", de0)
	}
	if byProgram["DE"][1].Duration != 60 || byProgram["DE"][1].MainExamer != "Palme, Frank" {
		t.Errorf("DE[1] wrong: %+v", byProgram["DE"][1])
	}
}

func TestParseJointCSVLatin1(t *testing.T) {
	// "Prüfungsform" / "mündlich" as ISO-8859-1 (ü = 0xFC)
	latin1 := "Nr;Modulname;Pr\xfcfungsform;Studiengruppe;Pr\xfcfungsplanung\n" +
		"5;M\xfcndliche;m\xfcndlich;DE;FK07\n"
	byProgram, err := ParseCSV(latin1)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(byProgram["DE"]) != 1 {
		t.Fatalf("want 1 DE row, got %d", len(byProgram["DE"]))
	}
	if byProgram["DE"][0].ExamType != "mündlich" {
		t.Errorf("latin1 decode failed: examType=%q", byProgram["DE"][0].ExamType)
	}
}

func TestParseJointCSVCommaAndMissingNr(t *testing.T) {
	csv := "Nr,Modulname,Studiengruppe,Prüfungsplanung\n" +
		"7,Test,GS,FK08\n" +
		",NoNr,GS,FK08\n" + // skipped (no Nr)
		"abc,BadNr,GS,FK08\n" // skipped (non-numeric)

	byProgram, err := ParseCSV(csv)
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

func TestParseJointCSVMissingRequiredColumn(t *testing.T) {
	if _, err := ParseCSV("Modulname,Studiengruppe\nMathe,DE\n"); err == nil {
		t.Error("expected error for missing Nr column")
	}
}

// A TAB-separated CSV with an empty field (e.g. IstWiederholung) must keep the row and
// its column alignment. With a whitespace delimiter, TrimLeadingSpace would collapse the
// empty field between two tabs and shift all later columns (dropping/misfiling the row).
func TestParseJointCSVEmptyFieldTabSeparated(t *testing.T) {
	csv := "Nr.int32()\tModulname.string()\tPrüfungsform.string()\tBewertung.string()\tDauer.int32()\tErstpruefender.string()\tZweitpruefender.string()\tIstWiederholung.string()\tStudiengruppe.string()\tPrüfungsplanung.string()\n" +
		"101\tComputational Thinking\tschrP\tbenotet\t90\tDietrich, Benedikt\tHobelsberger, Martin\tx\tID\tFK07\n" +
		"221\tMathematische Methoden\tschrP\tbenotet\t90\tBrockhaus, Sarah\tHögele, Wolfgang\t\tID\tFK07\n" +
		"301\tStatistik und Stochastik\tschrP\tbenotet\t60\tShao, Shuai\tBrockhaus, Sarah\tx\tID\tFK07\n"

	byProgram, err := ParseCSV(csv)
	if err != nil {
		t.Fatal(err)
	}
	if len(byProgram) != 1 || len(byProgram["ID"]) != 3 {
		t.Fatalf("expected 3 ID exams only, got %d programs / ID=%d", len(byProgram), len(byProgram["ID"]))
	}
	found := false
	for _, e := range byProgram["ID"] {
		if e.PrimussAncode == 221 {
			found = true
			if e.Module != "Mathematische Methoden" || e.Program != "ID" || e.Planer != "FK07" || e.IsRepeaterExam != "" {
				t.Errorf("221 columns shifted: module=%q program=%q planer=%q rep=%q",
					e.Module, e.Program, e.Planer, e.IsRepeaterExam)
			}
		}
	}
	if !found {
		t.Error("exam Nr 221 (empty IstWiederholung) was dropped")
	}
}
