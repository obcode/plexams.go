package main

import (
	"fmt"

	"github.com/obcode/plexams.go/graph/model"
)

// The converters from Mongo documents to the model types the PG writers take.
//
// Each one returns the value plus the notes worth showing the operator: dropped
// keys, recovered legacy spellings, and missing required fields. Nothing is
// rejected -- a refusing importer would leave the operator with 150 documents
// and no way to see which ones are the problem. Reporting and importing is the
// combination that lets them fix the few in the GUI afterwards.

type note struct {
	Kind    string // collection
	Key     string // the document's identity, never a name or mtknr
	Message string
}

// convertRoom maps a rooms document.
//
// needsRequest is read and thrown away on purpose: in PostgreSQL it is a
// generated column (request_with <> 'NONE'). Storing it is what let the two
// spellings drift apart in the first place -- 9 of 37 rooms carry both keys.
func convertRoom(d *doc) (*model.Room, []note) {
	name := d.str("name")
	r := &model.Room{
		Name:     name,
		Seats:    d.integer("seats"),
		Handicap: d.boolean("handicap"),
		Lab:      d.boolean("lab"),
		// both spellings, camelCase first
		PlacesWithSocket: d.boolean("placesWithSocket", "placeswithsocket"),
		RequestWith:      model.RoomRequestType(d.str("requestwith", "requestWith")),
		RequestPriority:  d.integer("requestpriority", "requestPriority"),
		Exahm:            d.boolean("exahm"),
		Seb:              d.boolean("seb"),
		SebSeats:         d.intPtr("sebseats", "sebSeats"),
		HmebSeats:        d.intPtr("hmebSeats", "hmebseats"),
		Deactivated:      d.boolean("deactivated"),
		Hitzewert:        d.intPtr("hitzewert"),
	}
	d.ignore("needsRequest", "needsrequest") // derived in PG, never stored

	var notes []note
	if name == "" {
		notes = append(notes, note{"rooms", "?", "name fehlt -- Raum wird uebersprungen"})
	}
	if r.RequestWith == "" {
		r.RequestWith = model.RoomRequestTypeNone
		notes = append(notes, note{"rooms", name, "requestwith fehlt -> NONE"})
	}
	return r, notes
}

// convertNta maps an nta document.
//
// `group` is NOT dropped although model.NTA has no such field: it is the former
// name of `program`. The two never occur together (21 documents carry only
// `group`, 59 only `program`), so dropping it as an unknown key would lose the
// study program for a quarter of the NTAs -- and program is NOT NULL.
//
// `exams` and `notForExams` really are dropped. The model lost them years ago,
// so the running code has been ignoring them for just as long; carrying them
// over would restore an exclusion that has not been in effect.
func convertNta(d *doc) (*model.NTA, []note) {
	mtknr := d.str("mtknr")
	program := d.str("program")

	var notes []note
	if program == "" {
		if g := d.str("group"); g != "" {
			program = g
			notes = append(notes, note{"nta", mtknr, "program aus dem Altfeld `group` uebernommen: " + g})
		}
	} else {
		d.ignore("group")
	}

	// notForExams holds ancodes (numbers), so this checks the raw value rather than
	// asking for strings -- which would have quietly reported nothing.
	if v, ok := d.get("notForExams"); ok {
		if arr, isArr := asSlice(v); isArr && len(arr) > 0 {
			notes = append(notes, note{"nta", mtknr,
				fmt.Sprintf("notForExams %v verworfen (model.NTA kennt das Feld nicht)", arr)})
		}
	}
	d.ignore("exams")

	n := &model.NTA{
		Name:                 d.str("name"),
		Email:                d.strPtr("email"),
		Mtknr:                mtknr,
		Compensation:         d.str("compensation"),
		DeltaDurationPercent: d.integer("deltaDurationPercent"),
		NeedsRoomAlone:       d.boolean("needsRoomAlone"),
		NeedsHardware:        d.boolean("needsHardware"),
		Program:              program,
		From:                 d.str("from"),
		Until:                d.str("until"),
		LastSemester:         d.strPtr("lastSemester"),
		Deactivated:          d.boolean("deactivated"),
	}

	// The columns are NOT NULL, and the empty string satisfies that. These are old,
	// incomplete entries; importing them with a note beats skipping them, because a
	// skipped NTA is an accommodation someone silently stops getting.
	if n.Program == "" {
		notes = append(notes, note{"nta", mtknr, missing("nta", mtknr, "program (weder program noch group)")})
	}
	if n.From == "" || n.Until == "" {
		notes = append(notes, note{"nta", mtknr, "Gueltigkeit unvollstaendig (from/until leer)"})
	}
	return n, notes
}

// convertStudyProgram maps a study_programs document.
//
// It applies the legacy category upgrade mucdai -> joint + jointFaculty. That is
// not an invention here: plexams.migrateMucdaiToJoint does exactly this on every
// start. It has to happen at import time because the PG check constraint only
// allows fk07/joint/misc, so the legacy value cannot be written and then fixed --
// three of the fifteen programs would simply fail to import.
func convertStudyProgram(d *doc) (*model.StudyProgram, []note) {
	shortname := d.str("shortname")
	p := &model.StudyProgram{
		Shortname:         shortname,
		Name:              d.str("name"),
		Degree:            d.strPtr("degree"),
		ZpaCode:           d.str("zpaCode", "zpacode"),
		Category:          d.str("category"),
		Active:            d.boolean("active"),
		Retired:           d.boolean("retired"),
		ExternalExamsBase: d.intPtr("externalExamsBase", "externalexamsbase"),
		JointFaculty:      d.strPtr("jointFaculty", "jointfaculty"),
	}

	var notes []note
	if shortname == "" {
		notes = append(notes, note{"study_programs", "?", "shortname fehlt -- Studiengang wird uebersprungen"})
	}
	if p.Category == "mucdai" {
		p.Category = "joint"
		if p.JointFaculty == nil {
			mucDai := "MUC.DAI"
			p.JointFaculty = &mucDai
		}
		notes = append(notes, note{"study_programs", shortname,
			"Altkategorie `mucdai` -> `joint` + jointFaculty MUC.DAI (wie migrateMucdaiToJoint)"})
	}
	// The constraint is jointFaculty NOT NULL exactly for joint, so a joint program
	// without one would be rejected far away from its cause.
	if p.Category == "joint" && p.JointFaculty == nil {
		notes = append(notes, note{"study_programs", shortname,
			"joint ohne jointFaculty -- wird abgelehnt, bitte in der GUI setzen"})
	}
	return p, notes
}

func convertNonInvigilator(d *doc) (*model.PermanentNonInvigilator, []note) {
	id := d.integer("teacherid", "teacherID")
	n := &model.PermanentNonInvigilator{
		TeacherID:  id,
		Name:       d.str("name"),
		Reason:     d.str("reason"),
		ValidFrom:  d.strPtr("validFrom", "validfrom"),
		ValidUntil: d.strPtr("validUntil", "validuntil"),
	}
	var notes []note
	if id == 0 {
		notes = append(notes, note{"permanent_non_invigilators", "?", "teacherid fehlt -- Eintrag wird uebersprungen"})
	}
	return n, notes
}

func convertAnnyConfig(d *doc) *model.AnnyConfig {
	return &model.AnnyConfig{
		PersonalizationNames: d.strings("personalizationnames", "personalizationNames"),
	}
}
