package plexams

import "github.com/obcode/plexams.go/graph/model"

// This file is the GUI-facing catalog for the editable email templates: per template a
// human description, the variables (placeholders) it may use — each with a plain-language
// description and the value used in the live preview — plus representative sample data the
// preview renders against. It is the single source of truth behind the emailTemplates /
// emailTemplateFunctions / renderEmailTemplatePreview GraphQL fields, so a non-technical
// user can see which values exist, how to write them, and how the mail will look.

// samplePlanerName is the planner signature used across the preview sample data.
const samplePlanerName = "Prof. Dr. Max Planer"

// The following small typed sample structs are used where a template compares a pointer to
// nil ({{ if ne .ZpaStudent nil }}) — a plain map value can't express a typed nil pointer.
type (
	sampleZpaStudent struct{ Gender, Email string }
	sampleStudentReg struct {
		Name       string
		ZpaStudent *sampleZpaStudent
	}
	sampleNta struct {
		From, Compensation   string
		DeltaDurationPercent int
		NeedsRoomAlone       bool
		NeedsHardware        bool
	}
	sampleNTAStudent struct {
		Name       string
		ZpaStudent *sampleZpaStudent
		Nta        sampleNta
	}
)

// emailTemplateVar documents one placeholder for the GUI editor.
type emailTemplateVar struct {
	Name        string // how to write it, e.g. "{{ .Teacher.Fullname }}"
	Description string // plain-language meaning
	Example     string // the value used in the live preview
}

// emailTemplateInfo is the catalog entry for one editable template.
type emailTemplateInfo struct {
	Description string             // short purpose of the mail
	Jira        bool               // whether the shared "reply via JIRA" note is added
	Variables   []emailTemplateVar // the placeholders it may use
	Sample      any                // representative data for the live preview
}

// v is a small constructor to keep the catalog literal readable.
func v(name, description, example string) emailTemplateVar {
	return emailTemplateVar{Name: name, Description: description, Example: example}
}

// modelVariables converts the catalog entry's documented variables to the GraphQL model.
func (i emailTemplateInfo) modelVariables() []*model.EmailTemplateVariable {
	out := make([]*model.EmailTemplateVariable, 0, len(i.Variables))
	for _, vr := range i.Variables {
		out = append(out, &model.EmailTemplateVariable{Name: vr.Name, Description: vr.Description, Example: vr.Example})
	}
	return out
}

// emailTemplateFuncDocs documents the helper functions available in every email template.
// The list is global — all functions may be used in any template.
var emailTemplateFuncDocs = []*model.EmailTemplateFunction{
	{
		Name:        "jiraURL",
		Usage:       "{{ jiraURL }}",
		Description: "URL des JIRA-Servicedesks der Prüfungsplanung (aus der Konfiguration).",
	},
	{
		Name:        "zpaURL",
		Usage:       "{{ zpaURL }}",
		Description: "Basis-URL des ZPA (ohne abschließenden Slash), z. B. für Links auf den Prüfungsplan.",
	},
	{
		Name:        "plural",
		Usage:       "{{ plural .Minutes \"Minute\" \"Minuten\" }}",
		Description: "Formatiert eine Zahl mit dem passenden Singular/Plural, z. B. „1 Minute“ bzw. „5 Minuten“.",
	},
	{
		Name:        "constraintsText",
		Usage:       "{{ constraintsText .Constraints }}",
		Description: "Wandelt die Besonderheiten/Constraints einer Prüfung in lesbaren Text, z. B. „EXaHM; Labor; Steckdosen“. Leer, wenn keine.",
	},
	{
		Name:        "add",
		Usage:       "{{ add $index 1 }}",
		Description: "Addiert zwei ganze Zahlen (praktisch für Nummerierungen in Aufzählungen).",
	},
}

// emailTemplateCatalog maps each editable template's file name to its GUI catalog entry.
// Keep the keys in sync with the *.md.tmpl files in plexams/email/tmpl.
var emailTemplateCatalog = map[string]emailTemplateInfo{
	"assembledExamEmail.md.tmpl": {
		Description: "An die/den Prüfende:n: Bestätigung der zusammengestellten Anmeldedaten einer Prüfung (mit CSV/Markdown im Anhang), inkl. Nachteilsausgleiche.",
		Jira:        true,
		Variables: []emailTemplateVar{
			v("{{ .Teacher.Fullname }}", "Voller Name der/des Prüfenden.", "Prof. Dr. Erika Mustermann"),
			v("{{ .HasStudentRegs }}", "Ob überhaupt Anmeldungen vorliegen (wahr/falsch).", "true"),
			v("{{ .Exam.ZpaExam.AnCode }}", "Ancode (Prüfungsnummer) im ZPA.", "1234"),
			v("{{ .Exam.ZpaExam.Module }}", "Modulname der Prüfung.", "Datenbanken"),
			v("{{ .FromDate }}", "Beginn des Prüfungszeitraums.", "10.07.2026"),
			v("{{ .ToDate }}", "Ende des Prüfungszeitraums.", "24.07.2026"),
			v("{{ range .Exam.PrimussExams }}", "Schleife über die Primuss-Prüfungen (je Studiengang).", "1 Studiengang"),
			v("{{ .Exam.Program }}", "Studiengangskürzel (innerhalb der Schleife).", "IF"),
			v("{{ range .StudentRegs }}", "Schleife über die Anmeldungen eines Studiengangs.", "2 Anmeldungen"),
			v("{{ .Name }}", "Name der/des angemeldeten Studierenden.", "Anna Beispiel"),
			v("{{ .ZpaStudent.Gender }}", "Geschlecht laut ZPA (nur wenn im ZPA vorhanden).", "w"),
			v("{{ .ZpaStudent.Email }}", "E-Mail-Adresse laut ZPA.", "anna@hm.edu"),
			v("{{ range .Ntas }}", "Schleife über Studierende mit Nachteilsausgleich.", "1 NTA"),
			v("{{ .Compensation }}", "Art des Nachteilsausgleichs.", "25% Zeitverlängerung"),
			v("{{ .PlanerName }}", "Name der/des Planenden (Unterschrift).", samplePlanerName),
		},
		Sample: map[string]any{
			"Teacher":        map[string]any{"Fullname": "Prof. Dr. Erika Mustermann"},
			"HasStudentRegs": true,
			"FromDate":       "10.07.2026",
			"ToDate":         "24.07.2026",
			"PlanerName":     samplePlanerName,
			"Exam": map[string]any{
				"ZpaExam": map[string]any{"AnCode": 1234, "Module": "Datenbanken"},
				"PrimussExams": []any{
					map[string]any{
						"Exam": map[string]any{"Program": "IF"},
						"StudentRegs": []sampleStudentReg{
							{Name: "Anna Beispiel", ZpaStudent: &sampleZpaStudent{Gender: "w", Email: "anna@hm.edu"}},
							{Name: "Ben Muster", ZpaStudent: nil},
						},
						"Ntas": []any{
							map[string]any{"Name": "Anna Beispiel", "Compensation": "25% Zeitverlängerung"},
						},
					},
				},
			},
		},
	},

	"coverPageEmail.md.tmpl": {
		Description: "An die/den Prüfende:n: die generierten Deckblätter für ihre/seine Prüfungen im Anhang.",
		Jira:        false,
		Variables: []emailTemplateVar{
			v("{{ .Teacher.Fullname }}", "Voller Name der/des Prüfenden.", "Prof. Dr. Erika Mustermann"),
			v("{{ .GeneratorName }}", "Name des Deckblatt-Generators.", "Plexams"),
			v("{{ .PlanerName }}", "Name der/des Planenden (Unterschrift).", samplePlanerName),
		},
		Sample: map[string]any{
			"Teacher":       map[string]any{"Fullname": "Prof. Dr. Erika Mustermann"},
			"GeneratorName": "Plexams",
			"PlanerName":    samplePlanerName,
		},
	},

	"draftEmailFS.md.tmpl": {
		Description: "An die Fachschaft: der vorläufige Prüfungsplan als PDF, mit Rückmeldefrist.",
		Jira:        false,
		Variables: []emailTemplateVar{
			v("{{ .FeedbackDate }}", "Frist für Rückmeldungen.", "03.07.2026"),
			v("{{ .PlanerName }}", "Name der/des Planenden (Unterschrift).", samplePlanerName),
		},
		Sample: map[string]any{"FeedbackDate": "03.07.2026", "PlanerName": samplePlanerName},
	},

	"draftEmailZPA.md.tmpl": {
		Description: "An die Prüfenden: die vorläufige Planung ist im ZPA sichtbar, mit Rückmeldefrist.",
		Jira:        true,
		Variables: []emailTemplateVar{
			v("{{ .FeedbackDate }}", "Frist für Rückmeldungen.", "03.07.2026"),
			v("{{ .FromDate }}", "Beginn der Prüfungen der FK07.", "10.07.2026"),
			v("{{ .PlanerName }}", "Name der/des Planenden (Unterschrift).", samplePlanerName),
		},
		Sample: map[string]any{"FeedbackDate": "03.07.2026", "FromDate": "10.07.2026", "PlanerName": samplePlanerName},
	},

	"exahmEmail.md.tmpl": {
		Description: "An alle Prüfenden: Aufruf, EXaHM-/SEB-Prüfungen frühzeitig per Ticket zu melden (Raumbedarf T-Bau).",
		Jira:        true,
		Variables: []emailTemplateVar{
			v("{{ .PlanerName }}", "Name der/des Planenden (Unterschrift).", samplePlanerName),
		},
		Sample: map[string]any{"PlanerName": samplePlanerName},
	},

	"examPlanningInfoEmail.md.tmpl": {
		Description: "An die/den Prüfende:n: welche ihrer/seiner Prüfungen wir planen (oder keine), mit Bitte um Besonderheiten/Constraints.",
		Jira:        true,
		Variables: []emailTemplateVar{
			v("{{ .Teacher.Fullname }}", "Voller Name der/des Prüfenden.", "Prof. Dr. Erika Mustermann"),
			v("{{ .Category }}", "\"withExams\" wenn wir Prüfungen planen, sonst leer/anderes.", "withExams"),
			v("{{ .FromDate }}", "Beginn des Prüfungszeitraums.", "10.07.2026"),
			v("{{ .UntilDate }}", "Ende des Prüfungszeitraums.", "24.07.2026"),
			v("{{ range .Exams }}", "Schleife über die geplanten Prüfungen.", "2 Prüfungen"),
			v("{{ .Ancode }}", "Ancode (Prüfungsnummer).", "1234"),
			v("{{ .Module }}", "Modulname.", "Datenbanken"),
			v("{{ constraintsText .Constraints }}", "Besonderheiten der Prüfung als Text (siehe Funktionen).", "EXaHM; Labor"),
			v("{{ .PlanerName }}", "Name der/des Planenden (Unterschrift).", samplePlanerName),
		},
		Sample: map[string]any{
			"Teacher":    map[string]any{"Fullname": "Prof. Dr. Erika Mustermann"},
			"Category":   "withExams",
			"FromDate":   "10.07.2026",
			"UntilDate":  "24.07.2026",
			"PlanerName": samplePlanerName,
			"Exams": []any{
				map[string]any{
					"Ancode":      1234,
					"Module":      "Datenbanken",
					"Constraints": &model.Constraints{RoomConstraints: &model.RoomConstraints{Exahm: true, Lab: true}},
				},
				map[string]any{
					"Ancode":      1250,
					"Module":      "Software Engineering",
					"Constraints": (*model.Constraints)(nil),
				},
			},
		},
	},

	"handicapEmailPlanned.md.tmpl": {
		Description: "An eine:n Studierende:n mit Nachteilsausgleich: in welchem Raum sie/er je Prüfung eingeplant ist.",
		Jira:        false,
		Variables: []emailTemplateVar{
			v("{{ .NTA.Name }}", "Name der/des Studierenden.", "Studi Beispiel"),
			v("{{ range .ExamsWithRoom }}", "Schleife über die eingeplanten Prüfungen mit Raum.", "1 Prüfung"),
			v("{{ .Date }}", "Datum der Prüfung.", "12.07.2026"),
			v("{{ .Time }}", "Uhrzeit der Prüfung.", "10:30"),
			v("{{ .Room.RoomName }}", "Raumname.", "R1.234"),
			v("{{ .Exam.ZpaExam.MainExamer }}", "Prüfende:r.", "Prof. Mustermann"),
			v("{{ .Exam.ZpaExam.Module }}", "Modulname.", "Datenbanken"),
			v("{{ .Waiver }}", "Verzicht auf eigenen Raum (Text), sonst leer.", "(leer)"),
			v("{{ .PlanerName }}", "Name der/des Planenden (Unterschrift).", samplePlanerName),
		},
		Sample: map[string]any{
			"NTA":        map[string]any{"Name": "Studi Beispiel"},
			"PlanerName": samplePlanerName,
			"ExamsWithRoom": []any{
				map[string]any{
					"Date":   "12.07.2026",
					"Time":   "10:30",
					"Room":   map[string]any{"RoomName": "R1.234"},
					"Exam":   map[string]any{"ZpaExam": map[string]any{"MainExamer": "Prof. Mustermann", "Module": "Datenbanken"}},
					"Waiver": "",
				},
			},
		},
	},

	"handicapEmailRoomAlone.md.tmpl": {
		Description: "An eine:n Studierende:n mit Anspruch auf eigenen Raum: für welche Prüfungen ein extra Raum geplant wird.",
		Jira:        false,
		Variables: []emailTemplateVar{
			v("{{ .NTA.Name }}", "Name der/des Studierenden.", "Studi Beispiel"),
			v("{{ range .Exams }}", "Schleife über die betroffenen Prüfungen.", "1 Prüfung"),
			v("{{ .ZpaExam.MainExamer }}", "Prüfende:r.", "Prof. Mustermann"),
			v("{{ .ZpaExam.Module }}", "Modulname.", "Datenbanken"),
			v("{{ .PlanerName }}", "Name der/des Planenden (Unterschrift).", samplePlanerName),
		},
		Sample: map[string]any{
			"NTA":        map[string]any{"Name": "Studi Beispiel"},
			"PlanerName": samplePlanerName,
			"Exams": []any{
				map[string]any{"ZpaExam": map[string]any{"MainExamer": "Prof. Mustermann", "Module": "Datenbanken"}},
			},
		},
	},

	"invigilationEmail.md.tmpl": {
		Description: "An das Kollegium: Aufruf, die „Anforderungen an die Aufsichtenplanung“ im ZPA einzutragen, mit Hintergrundinfos.",
		Jira:        true,
		Variables: []emailTemplateVar{
			v("{{ .FeedbackDate }}", "Frist für die Eingabe der Anforderungen.", "03.07.2026"),
			v("{{ .PlanerName }}", "Name der/des Planenden (Unterschrift).", samplePlanerName),
		},
		Sample: map[string]any{"FeedbackDate": "03.07.2026", "PlanerName": samplePlanerName},
	},

	"invigilationMissingEmail.md.tmpl": {
		Description: "An eine:n Prüfende:n: Erinnerung, dass die Aufsichts-Anforderungen im ZPA noch fehlen (sonst volles Deputat).",
		Jira:        true,
		Variables: []emailTemplateVar{
			v("{{ .Teacher.Fullname }}", "Voller Name der/des Prüfenden.", "Prof. Dr. Erika Mustermann"),
			v("{{ .Semester }}", "Semesterbezeichnung.", "Sommersemester 2026"),
			v("{{ .Minutes }}", "Volles Aufsichtsdeputat in Minuten.", "1200"),
			v("{{ .PlanerName }}", "Name der/des Planenden (Unterschrift).", samplePlanerName),
		},
		Sample: map[string]any{
			"Teacher":    map[string]any{"Fullname": "Prof. Dr. Erika Mustermann"},
			"Semester":   "Sommersemester 2026",
			"Minutes":    1200,
			"PlanerName": samplePlanerName,
		},
	},

	"invigilationsSecretariatEmail.md.tmpl": {
		Description: "An das Sekretariat: die Prüfungsplanung ist abgeschlossen und im ZPA hinterlegt, der Plan kann ausgehängt werden.",
		Jira:        false,
		Variables: []emailTemplateVar{
			v("{{ .SemesterName }}", "Semesterbezeichnung.", "Sommersemester 2026"),
			v("{{ .PlanerName }}", "Name der/des Planenden (Unterschrift).", samplePlanerName),
		},
		Sample: map[string]any{"SemesterName": "Sommersemester 2026", "PlanerName": samplePlanerName},
	},

	"kdpExahmEmail.md.tmpl": {
		Description: "An das KDP-Team: Übersicht der Räume mit EXaHM-/SEB-Prüfungen, nach Tag/Zeit und Raum, mit Platzaufteilung.",
		Jira:        false,
		Variables: []emailTemplateVar{
			v("{{ .SemesterName }}", "Semesterbezeichnung.", "Sommersemester 2026"),
			v("{{ range .Slots }}", "Schleife über die Slots (Tag/Zeit).", "1 Slot"),
			v("{{ .Date }}", "Datum des Slots.", "12.07.2026"),
			v("{{ .Time }}", "Uhrzeit des Slots.", "10:30"),
			v("{{ range .Rooms }}", "Schleife über die Räume eines Slots.", "1 Raum"),
			v("{{ .RoomName }}", "Raumname.", "T2.017"),
			v("{{ range .Exams }}", "Schleife über die Prüfungen eines Raums.", "1 Prüfung"),
			v("{{ .Ancode }}", "Ancode (Prüfungsnummer).", "1234"),
			v("{{ .Module }}", "Modulname.", "Datenbanken"),
			v("{{ .Examer }}", "Prüfende:r.", "Prof. Mustermann"),
			v("{{ .Type }}", "Prüfungssystem (EXaHM oder SEB).", "EXaHM"),
			v("{{ .Seats }}", "Anzahl der Plätze (für plural).", "30"),
			v("{{ .Detail }}", "Zusatzinfo zur Prüfung.", "…"),
			v("{{ .PlanerName }}", "Name der/des Planenden (Unterschrift).", samplePlanerName),
		},
		Sample: map[string]any{
			"SemesterName": "Sommersemester 2026",
			"PlanerName":   samplePlanerName,
			"Slots": []any{
				map[string]any{
					"Date": "12.07.2026",
					"Time": "10:30",
					"Rooms": []any{
						map[string]any{
							"RoomName": "T2.017",
							"Exams": []any{
								map[string]any{"Ancode": 1234, "Module": "Datenbanken", "Examer": "Prof. Mustermann", "Type": "EXaHM", "Seats": 30, "Detail": "vollständig"},
							},
						},
					},
				},
			},
		},
	},

	"lbaRepeaterEmail.md.tmpl": {
		Description: "Interne Übersicht: von LBAs geplante Wiederholungsprüfungen mit Terminen und Aufsichten (Aufsichten in Cc).",
		Jira:        false,
		Variables: []emailTemplateVar{
			v("{{ .SemesterName }}", "Semesterbezeichnung.", "Sommersemester 2026"),
			v("{{ range .Exams }}", "Schleife über die Prüfungen.", "1 Prüfung"),
			v("{{ .Module }}", "Modulname.", "Datenbanken"),
			v("{{ .Examer.Name }}", "Name der/des Prüfenden (LBA).", "LB Beispiel"),
			v("{{ .Examer.Email }}", "E-Mail der/des Prüfenden.", "lb@hm.edu"),
			v("{{ .Date }}", "Termin (Datum).", "12.07.2026"),
			v("{{ .Time }}", "Termin (Uhrzeit), optional.", "10:30"),
			v("{{ range .Programs }}", "Schleife über die Studiengänge mit Anmeldezahlen.", "2 Studiengänge"),
			v("{{ .Name }} / {{ .Count }}", "Studiengangskürzel und Anmeldezahl.", "IF: 12"),
			v("{{ range .Invigilators }}", "Schleife über die geplanten Aufsichten.", "1 Aufsicht"),
			v("{{ .PlanerName }}", "Name der/des Planenden (Unterschrift).", samplePlanerName),
		},
		Sample: map[string]any{
			"SemesterName": "Sommersemester 2026",
			"PlanerName":   samplePlanerName,
			"Exams": []any{
				map[string]any{
					"Module": "Datenbanken",
					"Examer": map[string]any{"Name": "LB Beispiel", "Email": "lb@hm.edu"},
					"Date":   "12.07.2026",
					"Time":   "10:30",
					"Programs": []any{
						map[string]any{"Name": "IF", "Count": 12},
						map[string]any{"Name": "DE", "Count": 3},
					},
					"Invigilators": []any{
						map[string]any{"Name": "Prof. Aufsicht", "Email": "aufsicht@hm.edu"},
					},
				},
			},
		},
	},

	"newNTAEmail.md.tmpl": {
		Description: "An die Prüfenden: Info über eine:n Studierende:n mit neuem Nachteilsausgleich und dessen Auswirkung auf die Planung.",
		Jira:        false,
		Variables: []emailTemplateVar{
			v("{{ .Student.Name }}", "Name der/des Studierenden.", "Studi Beispiel"),
			v("{{ .Student.ZpaStudent.Gender }}", "Geschlecht laut ZPA (nur wenn vorhanden).", "d"),
			v("{{ .Student.ZpaStudent.Email }}", "E-Mail laut ZPA.", "studi@hm.edu"),
			v("{{ .Student.Nta.From }}", "Datum des Nachteilsausgleichs.", "01.06.2026"),
			v("{{ .Student.Nta.Compensation }}", "Wortlaut des Nachteilsausgleichs.", "25% Zeitverlängerung"),
			v("{{ .Student.Nta.DeltaDurationPercent }}", "Prozentuale Zeitverlängerung.", "25"),
			v("{{ .Student.Nta.NeedsRoomAlone }}", "Braucht eigenen Raum (wahr/falsch).", "false"),
			v("{{ .Student.Nta.NeedsHardware }}", "Braucht besondere Hardware (wahr/falsch).", "false"),
			v("{{ range .Exams }}", "Schleife über die angemeldeten, geplanten Prüfungen.", "1 Prüfung"),
			v("{{ .Ancode }}", "Ancode (Prüfungsnummer).", "1234"),
			v("{{ .ZpaExam.Module }}", "Modulname.", "Datenbanken"),
			v("{{ .ZpaExam.MainExamer }}", "Prüfende:r.", "Prof. Mustermann"),
			v("{{ .PlanerName }}", "Name der/des Planenden (Unterschrift).", samplePlanerName),
		},
		Sample: map[string]any{
			"Student": sampleNTAStudent{
				Name:       "Studi Beispiel",
				ZpaStudent: &sampleZpaStudent{Gender: "d", Email: "studi@hm.edu"},
				Nta:        sampleNta{From: "01.06.2026", Compensation: "25% Zeitverlängerung", DeltaDurationPercent: 25},
			},
			"PlanerName": samplePlanerName,
			"Exams": []any{
				map[string]any{"Ancode": 1234, "ZpaExam": map[string]any{"Module": "Datenbanken", "MainExamer": "Prof. Mustermann"}},
			},
		},
	},

	"publishedEmailExams.md.tmpl": {
		Description: "An Prüfende und Fachschaft: der Prüfungsplan ist im ZPA veröffentlicht (Räume folgen).",
		Jira:        true,
		Variables: []emailTemplateVar{
			v("{{ .PlanerName }}", "Name der/des Planenden (Unterschrift).", samplePlanerName),
		},
		Sample: map[string]any{"PlanerName": samplePlanerName},
	},

	"publishedInvigilationPersonalEmail.md.tmpl": {
		Description: "An eine:n Prüfende:n: persönlicher Aufsichtenplan (PNG/ICS im Anhang) plus Transparenz-Statistik.",
		Jira:        true,
		Variables: []emailTemplateVar{
			v("{{ .Teacher.Fullname }}", "Voller Name der/des Prüfenden.", "Prof. Dr. Erika Mustermann"),
			v("{{ .NoOfInvigilators }}", "Anzahl der Aufsichten (für plural).", "5"),
			v("{{ .InvigilationInRooms }}", "Minuten Aufsicht in Räumen.", "600"),
			v("{{ .ReserveInvigilation }}", "Minuten Reserveaufsicht.", "120"),
			v("{{ .OtherContributions }}", "Anrechenbare Minuten (Beisitz etc.).", "90"),
			v("{{ .TodoPerInvigilator }}", "100%-Deputat in Minuten.", "1200"),
			v("{{ .MaxDeviation }}", "Minuten „zu wenig“ (Spanne).", "30"),
			v("{{ .MinDeviation }}", "Minuten „zu viel“ (Spanne).", "45"),
			v("{{ .PlanerName }}", "Name der/des Planenden (Unterschrift).", samplePlanerName),
		},
		Sample: map[string]any{
			"Teacher":             map[string]any{"Fullname": "Prof. Dr. Erika Mustermann"},
			"NoOfInvigilators":    5,
			"InvigilationInRooms": 600,
			"ReserveInvigilation": 120,
			"OtherContributions":  90,
			"TodoPerInvigilator":  1200,
			"MaxDeviation":        30,
			"MinDeviation":        45,
			"PlanerName":          samplePlanerName,
		},
	},

	"publishedRoomsPersonalEmail.md.tmpl": {
		Description: "An eine:n Prüfende:n: die veröffentlichten Räume ihrer/seiner Prüfungen, mit Belegung, Reserve, NTAs und Mitnutzung.",
		Jira:        true,
		Variables: []emailTemplateVar{
			v("{{ .Teacher.Shortname }}", "Kürzel der/des Prüfenden.", "must"),
			v("{{ range .Exams }}", "Schleife über die Prüfungen.", "1 Prüfung"),
			v("{{ .Ancode }}", "Ancode (Prüfungsnummer).", "1234"),
			v("{{ .Module }}", "Modulname.", "Datenbanken"),
			v("{{ .Date }}", "Datum.", "12.07.2026"),
			v("{{ .Time }}", "Uhrzeit.", "10:30"),
			v("{{ range .Rooms }}", "Schleife über die Räume einer Prüfung.", "1 Raum"),
			v("{{ .RoomName }}", "Raumname.", "R1.234"),
			v("{{ .Allocations }}", "Liste der Belegungszeilen (Studierende, Reserve, NTA).", "20 Studierende; +2 Reserve"),
			v("{{ range .SharedWith }}", "Schleife über mitnutzende Prüfungen.", "1 Prüfung"),
			v("{{ .ExamHeader }}", "Überschrift der mitnutzenden Prüfung.", "1250. Software Engineering"),
			v("{{ .PlanerName }}", "Name der/des Planenden (Unterschrift).", samplePlanerName),
		},
		Sample: map[string]any{
			"Teacher":    map[string]any{"Shortname": "must"},
			"PlanerName": samplePlanerName,
			"Exams": []any{
				map[string]any{
					"Ancode": 1234, "Module": "Datenbanken", "Date": "12.07.2026", "Time": "10:30",
					"Rooms": []any{
						map[string]any{
							"RoomName":    "R1.234",
							"Allocations": []string{"20 Studierende", "+2 Reserve"},
							"SharedWith": []any{
								map[string]any{"ExamHeader": "1250. Software Engineering (Prof. Kollege)", "Allocations": []string{"10 Studierende"}},
							},
						},
					},
				},
			},
		},
	},

	"roomRequestEmail.md.tmpl": {
		Description: "An das Gebäudemanagement: Raumanforderung mit Tagen und Zeiten (inkl. Vor-/Nachlauf).",
		Jira:        false,
		Variables: []emailTemplateVar{
			v("{{ .SemesterName }}", "Semesterbezeichnung.", "Sommersemester 2026"),
			v("{{ range .Rooms }}", "Schleife über die Räume.", "1 Raum"),
			v("{{ .Room }}", "Raumname.", "R1.234"),
			v("{{ range .Days }}", "Schleife über die Tage eines Raums.", "1 Tag"),
			v("{{ .Date }}", "Datum.", "12.07.2026"),
			v("{{ range .Times }}", "Schleife über die Zeitfenster eines Tages.", "1 Zeit"),
			v("{{ .From }} / {{ .Until }}", "Beginn und Ende des Zeitfensters.", "10:15 – 12:00"),
			v("{{ .PlanerName }}", "Name der/des Planenden (Unterschrift).", samplePlanerName),
		},
		Sample: roomOccupancySample(),
	},

	"roomsSecretariatEmail.md.tmpl": {
		Description: "An das Sekretariat: die eingeplanten Räume mit Belegungszeiten, zum Abgleich mit dem ZPA.",
		Jira:        false,
		Variables: []emailTemplateVar{
			v("{{ .SemesterName }}", "Semesterbezeichnung.", "Sommersemester 2026"),
			v("{{ range .Rooms }}", "Schleife über die Räume.", "1 Raum"),
			v("{{ .Room }}", "Raumname.", "R1.234"),
			v("{{ range .Days }}", "Schleife über die Tage eines Raums.", "1 Tag"),
			v("{{ .Date }}", "Datum.", "12.07.2026"),
			v("{{ range .Times }}", "Schleife über die Zeitfenster eines Tages.", "1 Zeit"),
			v("{{ .From }} / {{ .Until }}", "Beginn und Ende des Zeitfensters.", "10:15 – 12:00"),
			v("{{ .PlanerName }}", "Name der/des Planenden (Unterschrift).", samplePlanerName),
		},
		Sample: roomOccupancySample(),
	},

	"unplannedExamEmail.md.tmpl": {
		Description: "An die/den Prüfende:n einer nicht von uns geplanten Prüfung: die Anmeldedaten im Anhang.",
		Jira:        false,
		Variables: []emailTemplateVar{
			v("{{ .Exam.MainExamer }}", "Prüfende:r (Name aus den Primuss-Daten).", "Prof. Mustermann"),
			v("{{ .Exam.AnCode }}", "Ancode (Prüfungsnummer).", "1234"),
			v("{{ .Exam.Module }}", "Modulname.", "Datenbanken"),
			v("{{ .Exam.Program }}", "Studiengangskürzel.", "IF"),
			v("{{ .PlanerName }}", "Name der/des Planenden (Unterschrift).", samplePlanerName),
		},
		Sample: map[string]any{
			"Exam":       map[string]any{"MainExamer": "Prof. Mustermann", "AnCode": 1234, "Module": "Datenbanken", "Program": "IF"},
			"PlanerName": samplePlanerName,
		},
	},
}

// roomOccupancySample is the shared preview data for the room-request / rooms-secretariat
// mails, which use the same {Rooms -> Days -> Times} shape.
func roomOccupancySample() map[string]any {
	return map[string]any{
		"SemesterName": "Sommersemester 2026",
		"PlanerName":   samplePlanerName,
		"Rooms": []any{
			map[string]any{
				"Room": "R1.234",
				"Days": []any{
					map[string]any{
						"Date":  "12.07.2026",
						"Times": []any{map[string]any{"From": "10:15", "Until": "12:00"}},
					},
				},
			},
		},
	}
}
