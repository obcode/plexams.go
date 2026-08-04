package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
)

// The two collections of a semester database that are carried over. Everything
// else is re-imported from ZPA/Primuss/Anny -- see the package comment.
const (
	fileSemesterConfig = "semester_config_input"
	filePreplanExams   = "preplan_exams"
)

// importSemester registers the semester and moves its hand-entered contents.
//
// Registering it here is what createSemester does in the GUI: the registry row
// and the configuration belong together, and every semester-scoped table has a
// foreign key onto that row. Doing it in the GUI first would work too, but then
// the config would be written twice -- once empty by hand, once from the dump.
func importSemester(ctx context.Context, pg *db.PG, o options, semester string, rep *report) error {
	fmt.Printf("\nSemester-Dump: %s  (Semester %s)\n\n", o.semesterDump, semester)

	if !o.dryRun {
		if err := pg.EnsureSemester(ctx, semester, db.CurrentSchemaVersion); err != nil {
			return fmt.Errorf("cannot register semester %s: %w", semester, err)
		}
	}

	if err := importSemesterConfig(ctx, pg, o, semester, rep); err != nil {
		return err
	}
	if err := importPreplanExams(ctx, pg, o, rep); err != nil {
		return err
	}

	reportUntouchedCollections(o.semesterDump, rep)
	return nil
}

func importSemesterConfig(ctx context.Context, pg *db.PG, o options, semester string, rep *report) error {
	docs, err := readBSON(filepath.Join(o.semesterDump, fileSemesterConfig+".bson"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Printf("  %-28s nicht im Dump -- Semester bleibt ohne Konfiguration\n", fileSemesterConfig)
			rep.note(note{fileSemesterConfig, semester,
				"keine Konfiguration im Dump -- in der GUI anlegen, sonst hat der Plan keine Termine"})
			return nil
		}
		return fmt.Errorf("%s: %w", fileSemesterConfig, err)
	}
	if len(docs) == 0 {
		return nil
	}
	// There is exactly one configuration per semester; a second document would be
	// an artefact of an old write path, and the last one written is the live one.
	if len(docs) > 1 {
		rep.note(note{fileSemesterConfig, semester,
			fmt.Sprintf("%d Konfigurationen im Dump -- die letzte gewinnt", len(docs))})
	}

	d := newDoc(docs[len(docs)-1])
	cfg, notes := convertSemesterConfigInput(d, jointProgramNames(ctx, pg, o, rep))
	rep.note(notes...)
	rep.drop(fileSemesterConfig, d.leftovers())

	if !o.dryRun {
		if err := pg.SaveSemesterConfigInputFor(ctx, semester, cfg); err != nil {
			return fmt.Errorf("cannot save the semester config: %w", err)
		}
	}
	rep.imported[fileSemesterConfig]++
	fmt.Printf("  %-28s %3d gelesen, %3d importiert (%d Startzeiten, %d gesperrte Tage, %d joint-Programme)\n",
		fileSemesterConfig, len(docs), rep.imported[fileSemesterConfig],
		len(cfg.StartTimes), len(cfg.ForbiddenDays), len(cfg.JointProgramAllowedTimes))
	return nil
}

// jointProgramNames returns the shortnames of the joint study programs, needed to
// spread the legacy mucDaiAllowedTimes list.
//
// The database is asked, not the dump: by the time this runs the study programs
// are imported, and the database is the authority on which of them are joint --
// including any the planner fixed by hand in an earlier run. Only a dry run,
// which has written nothing, falls back to what this run's global dump held.
func jointProgramNames(ctx context.Context, pg *db.PG, o options, rep *report) []string {
	if o.dryRun {
		return rep.jointPrograms
	}
	programs, err := pg.StudyPrograms(ctx)
	if err != nil {
		rep.note(note{fileSemesterConfig, "config",
			fmt.Sprintf("Studiengaenge nicht lesbar (%v) -- reservierte Zeiten koennen nicht verteilt werden", err)})
		return nil
	}
	names := make([]string, 0)
	for _, p := range programs {
		if p.Category == "joint" {
			names = append(names, p.Shortname)
		}
	}
	return names
}

func importPreplanExams(ctx context.Context, pg *db.PG, o options, rep *report) error {
	docs, err := readBSON(filepath.Join(o.semesterDump, filePreplanExams+".bson"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Printf("  %-28s nicht im Dump -- uebersprungen\n", filePreplanExams)
			return nil
		}
		return fmt.Errorf("%s: %w", filePreplanExams, err)
	}

	exams := make([]*model.PreplanExam, 0, len(docs))
	for _, m := range docs {
		d := newDoc(m)
		e, notes := convertPreplanExam(d)
		rep.note(notes...)
		rep.drop(filePreplanExams, d.leftovers())
		if e.ID == 0 {
			rep.skipped[filePreplanExams]++
			continue
		}
		exams = append(exams, e)
	}
	rep.note(makePairsSymmetric(exams)...)

	if !o.dryRun {
		// Two passes. A pair is a foreign key onto the OTHER pre-exam, so the first
		// pass writes every exam without its pairs and the second adds them once all
		// the rows exist. The CSV import does the same for the same reason.
		for _, e := range exams {
			bare := *e
			bare.NotSameSlot, bare.CanShareSlot = nil, nil
			if err := pg.UpsertPreplanExam(ctx, &bare); err != nil {
				return fmt.Errorf("pre-exam %d: %w", e.ID, err)
			}
		}
		for _, e := range exams {
			if len(e.NotSameSlot) == 0 && len(e.CanShareSlot) == 0 {
				continue
			}
			if err := pg.UpsertPreplanExam(ctx, e); err != nil {
				return fmt.Errorf("pre-exam %d (Paare): %w", e.ID, err)
			}
		}
	}
	rep.imported[filePreplanExams] += len(exams)

	planned := 0
	for _, e := range exams {
		if e.PlannedStarttime != nil {
			planned++
		}
	}
	fmt.Printf("  %-28s %3d gelesen, %3d importiert, %3d uebersprungen (%d mit Termin)\n",
		filePreplanExams, len(docs), rep.imported[filePreplanExams], rep.skipped[filePreplanExams], planned)
	return nil
}

// makePairsSymmetric adds the missing counterpart of every one-sided pair.
//
// In MongoDB each pre-exam carried its own list and nothing forced the two to
// agree; in PostgreSQL a pair is ONE canonical row (a < b). Writing the exams one
// by one would therefore let a one-sided pair be deleted again by the write of
// the other exam, which replaces its whole side of the relation. Completing the
// pairs first makes the outcome independent of the order -- and reports the
// asymmetries, because a pair only one side knew about is a finding about the old
// data, not a detail.
func makePairsSymmetric(exams []*model.PreplanExam) []note {
	byID := make(map[int]*model.PreplanExam, len(exams))
	for _, e := range exams {
		byID[e.ID] = e
	}

	var notes []note
	add := func(from, to int, get func(*model.PreplanExam) *[]int, label string) {
		other, ok := byID[to]
		if !ok {
			notes = append(notes, note{filePreplanExams, fmt.Sprintf("#%d", from),
				fmt.Sprintf("%s verweist auf die unbekannte Vorplanungs-Pruefung %d -- Paar verworfen", label, to)})
			return
		}
		list := get(other)
		for _, id := range *list {
			if id == from {
				return
			}
		}
		*list = append(*list, from)
		notes = append(notes, note{filePreplanExams, fmt.Sprintf("#%d", from),
			fmt.Sprintf("%s %d war einseitig -- Gegenrichtung ergaenzt", label, to)})
	}

	for _, e := range exams {
		for _, other := range append([]int(nil), e.NotSameSlot...) {
			add(e.ID, other, func(p *model.PreplanExam) *[]int { return &p.NotSameSlot }, "notSameSlot")
		}
		for _, other := range append([]int(nil), e.CanShareSlot...) {
			add(e.ID, other, func(p *model.PreplanExam) *[]int { return &p.CanShareSlot }, "canShareSlot")
		}
	}

	// A pair pointing at an unknown id would be rejected by the foreign key, so it
	// is dropped here -- with the note above, never quietly.
	for _, e := range exams {
		e.NotSameSlot = keepKnown(e.NotSameSlot, byID)
		e.CanShareSlot = keepKnown(e.CanShareSlot, byID)
	}
	return notes
}

func keepKnown(ids []int, byID map[int]*model.PreplanExam) []int {
	out := ids[:0]
	for _, id := range ids {
		if _, ok := byID[id]; ok {
			out = append(out, id)
		}
	}
	return out
}

// reportUntouchedCollections lists what the semester dump holds beyond the two
// collections that are imported.
//
// This is the safety net for the assumption the whole cut-over rests on: that
// everything else can be fetched again. A semester that turns out to carry
// constraints, connected exams or a finished plan shows up here as a non-empty
// collection -- before anyone notices it missing in the GUI.
func reportUntouchedCollections(dumpDir string, rep *report) {
	entries, err := os.ReadDir(dumpDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".bson") {
			continue
		}
		collection := strings.TrimSuffix(name, ".bson")
		if collection == fileSemesterConfig || collection == filePreplanExams {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.Size() == 0 {
			continue
		}
		docs, err := readBSON(filepath.Join(dumpDir, name))
		if err != nil || len(docs) == 0 {
			continue
		}
		rep.untouched = append(rep.untouched, fmt.Sprintf("%-34s %6d Dokument(e)", collection, len(docs)))
	}
	sort.Strings(rep.untouched)
}
