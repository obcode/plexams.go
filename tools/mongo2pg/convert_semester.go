package main

import (
	"fmt"
	"time"

	"github.com/obcode/plexams.go/graph/model"
)

// Converters for the ONE semester that is carried over at cut-over.
//
// Only two collections are read: the semester configuration and the pre-planned
// exams. Everything else a semester database holds comes back from ZPA, Primuss
// or Anny by importing it again -- these two do not, because a person typed them.
//
// None of these model types has bson tags (they are gqlgen-generated), so the
// driver stored the Go field names lowercased. That is why nearly every read here
// names the lowercase spelling first and the camelCase one as a fallback: both
// exist in the data, depending on when the document was last written.

// convertSemesterConfigInput maps a semester_config_input document.
//
// jointPrograms are the shortnames of the joint study programs, read from the
// database *after* the global master data has been imported -- they are needed
// for the legacy mucDaiAllowedTimes conversion below.
func convertSemesterConfigInput(d *doc, jointPrograms []string) (*model.SemesterConfigInput, []note) {
	var notes []note
	const kind = "semester_config_input"

	cfg := &model.SemesterConfigInput{
		StartTimes:            d.strings("startTimes", "starttimes"),
		ForbiddenDays:         d.timeSlice("forbiddenDays", "forbiddendays"),
		Emails:                convertEmails(d.sub("emails")),
		ExamGapMinutes:        d.intPtr("examGapMinutes", "examgapminutes"),
		TimelagMin:            d.intPtr("timelagMin", "timelagmin"),
		NotTooCloseMinutes:    d.intPtr("notTooCloseMinutes", "nottoocloseminutes"),
		CrossCampusGapMinutes: d.intPtr("crossCampusGapMinutes", "crosscampusgapminutes"),
		MaxSeatsPerSlot:       d.intPtr("maxSeatsPerSlot", "maxseatsperslot"),
	}

	if from, ok := d.timeVal("from"); ok {
		cfg.From = from
	} else {
		notes = append(notes, note{kind, "config", missing(kind, "config", "from")})
	}
	if until, ok := d.timeVal("until"); ok {
		cfg.Until = until
	} else {
		notes = append(notes, note{kind, "config", missing(kind, "config", "until")})
	}

	for _, j := range d.subSlice("jointProgramAllowedTimes", "jointprogramallowedtimes") {
		cfg.JointProgramAllowedTimes = append(cfg.JointProgramAllowedTimes, &model.JointProgramTimes{
			Program:      j.str("program"),
			AllowedTimes: j.timeSlice("allowedTimes", "allowedtimes"),
		})
	}

	// The legacy single MUC.DAI list. It carries `json:"-"`, and the column is
	// jsonb -- so writing the model as it stands would drop the times silently.
	// This is the conversion the schema comment in 00002 asks for, and it is the
	// same rule loadSemesterConfig applies at runtime (applyLegacyJointTimes):
	// one list, applied to every joint program.
	legacy := d.timeSlice("mucDaiAllowedTimes", "mucdaiallowedtimes")
	if len(cfg.JointProgramAllowedTimes) == 0 && len(legacy) > 0 {
		for _, program := range jointPrograms {
			times := make([]time.Time, len(legacy))
			copy(times, legacy)
			cfg.JointProgramAllowedTimes = append(cfg.JointProgramAllowedTimes,
				&model.JointProgramTimes{Program: program, AllowedTimes: times})
		}
		if len(jointPrograms) == 0 {
			notes = append(notes, note{kind, "config", fmt.Sprintf(
				"%d reservierte MUC.DAI-Zeiten aus dem Altfeld, aber kein joint-Studiengang in der Datenbank"+
					" -- Zeiten gehen verloren, Studiengaenge zuerst importieren", len(legacy))})
		} else {
			notes = append(notes, note{kind, "config", fmt.Sprintf(
				"%d reservierte Zeiten aus dem Altfeld `mucDaiAllowedTimes` auf %d joint-Studiengaenge uebertragen (%v)",
				len(legacy), len(jointPrograms), jointPrograms)})
		}
	}

	// Pre-slotless configuration: day/slot ordinals instead of start times. The
	// times cannot be recovered from them -- the mapping lived in the code of that
	// version -- so this is reported rather than guessed at.
	if _, ok := d.get("slots"); ok {
		d.ignore("mucDaiSlots", "mucdaislots")
		notes = append(notes, note{kind, "config",
			"Vor-slotless-Konfiguration (`slots` statt `startTimes`): Startzeiten wurden NICHT uebernommen," +
				" bitte in der GUI eintragen"})
	}
	if len(cfg.StartTimes) == 0 {
		notes = append(notes, note{kind, "config", "keine Startzeiten -- der Plan hat sonst keinen einzigen Termin"})
	}

	return cfg, notes
}

// convertEmails maps the emails sub-document. A missing one yields an empty
// struct rather than nil: the GUI edits the addresses field by field, and a nil
// here would be a null in the jsonb that every reader has to guard against.
func convertEmails(d *doc) *model.Emails {
	if d == nil {
		return &model.Emails{}
	}
	return &model.Emails{
		Profs:            d.str("profs"),
		Lbas:             d.str("lbas"),
		LbasLastSemester: d.str("lbaslastsemester", "lbasLastSemester"),
		AdditionalExamer: d.strings("additionalexamer", "additionalExamer"),
		Fs:               d.str("fs"),
		Sekr:             d.str("sekr"),
		RoomManagement:   d.str("roommanagement", "roomManagement"),
		Kdp:              d.str("kdp"),
		Lbaba:            d.str("lbaba"),
	}
}

// convertPreplanExam maps a preplan_exams document.
//
// The pair fields (notSameSlot/canShareSlot) are carried over as they are; the
// importer writes every pre-exam once without them and once with, because in
// PostgreSQL a pair is a foreign key onto the other pre-exam -- which need not
// exist yet while the first pass is running.
func convertPreplanExam(d *doc) (*model.PreplanExam, []note) {
	const kind = "preplan_exams"
	id := d.integer("id")
	key := fmt.Sprintf("#%d", id)

	e := &model.PreplanExam{
		ID:               id,
		ExamKind:         d.str("examkind", "examKind"),
		ExamerID:         d.integer("examerid", "examerID"),
		ExamerName:       d.str("examername", "examerName"),
		Module:           d.str("module"),
		Programs:         d.strings("programs"),
		ExpectedStudents: d.integer("expectedstudents", "expectedStudents"),
		Duration:         d.intPtr("duration"),
		PlannedStarttime: d.timePtr("plannedstarttime", "plannedStarttime"),
		IsFixed:          d.boolean("isfixed", "isFixed"),
		NotSameSlot:      d.ints("notsameslot", "notSameSlot"),
		CanShareSlot:     d.ints("canshareslot", "canShareSlot"),
		Ancode:           d.intPtr("ancode"),
		Notes:            d.str("notes"),
		Constraints:      convertConstraints(d.sub("constraints")),
	}

	var notes []note
	if id == 0 {
		notes = append(notes, note{kind, "?", "id fehlt -- Vorplanungs-Pruefung wird uebersprungen"})
	}

	// Pre-slotless document. The exam itself is worth keeping -- module, examiner,
	// expected students and constraints are all still valid -- but its placement is
	// a day/slot pair whose absolute time is not stored anywhere. It comes in
	// unplaced, and the report says so per exam.
	if e.PlannedStarttime == nil {
		if day, ok := d.number("planneddaynumber", "plannedDayNumber"); ok {
			slot, _ := d.number("plannedslotnumber", "plannedSlotNumber")
			notes = append(notes, note{kind, key, fmt.Sprintf(
				"Vor-slotless-Dokument: Termin (Tag %d, Slot %d) nicht uebernommen, Pruefung ist ungeplant",
				day, slot)})
		}
	} else {
		d.ignore("planneddaynumber", "plannedDayNumber", "plannedslotnumber", "plannedSlotNumber")
	}

	return e, notes
}

// convertConstraints maps the constraints sub-document of a pre-exam.
//
// In a pre-exam the SameSlot ints are PRE-EXAM ids, not ancodes (see
// model.PreplanExam) -- they are copied over unchanged, exactly as the pair
// fields are.
func convertConstraints(d *doc) *model.Constraints {
	if d == nil {
		return nil
	}
	c := &model.Constraints{
		Ancode:             d.integer("ancode"),
		NotPlannedByMe:     d.boolean("notplannedbyme", "notPlannedByMe"),
		DoNotPublish:       d.boolean("donotpublish", "doNotPublish"),
		ExcludeDays:        d.timePtrSlice("excludedays", "excludeDays"),
		PossibleDays:       d.timePtrSlice("possibledays", "possibleDays"),
		FixedDay:           d.timePtr("fixedday", "fixedDay"),
		FixedTime:          d.timePtr("fixedtime", "fixedTime"),
		SameSlot:           d.ints("sameslot", "sameSlot"),
		Online:             d.boolean("online"),
		Location:           d.strPtr("location"),
		NotPlannedByMeInFk: d.strPtr("notplannedbymeinfk", "notPlannedByMeInFK"),
	}
	if rc := d.sub("roomconstraints", "roomConstraints"); rc != nil {
		c.RoomConstraints = &model.RoomConstraints{
			AllowedRooms:     rc.strings("allowedrooms", "allowedRooms"),
			PlacesWithSocket: rc.boolean("placeswithsocket", "placesWithSocket"),
			Lab:              rc.boolean("lab"),
			Exahm:            rc.boolean("exahm"),
			Seb:              rc.boolean("seb"),
			KdpJiraURL:       rc.strPtr("kdpjiraurl", "kdpJiraURL"),
			MaxStudents:      rc.intPtr("maxstudents", "maxStudents"),
			AdditionalSeats:  rc.intPtr("additionalseats", "additionalSeats"),
			PreExamMinutes:   rc.intPtr("preexamminutes", "preExamMinutes"),
			PostExamMinutes:  rc.intPtr("postexamminutes", "postExamMinutes"),
			Comments:         rc.strPtr("comments"),
		}
	}
	return c
}
