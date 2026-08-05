// Command mongo2pg imports the MongoDB data that nobody can fetch again into
// PostgreSQL.
//
// It is a ONE-OFF for the cut-over and is meant to be deleted afterwards. It
// lives in its own module so that go.mongodb.org/mongo-driver does not return to
// the main one, which the migration just finished removing.
//
// It reads the ARCHIVED DUMP (mongodump's .bson files), not a live server. That
// makes a rehearsal reproducible and possible without VPN or a running mongod,
// and the dump is the artifact the migration plan keeps anyway.
//
// Two things are imported, and the rule for both is the same: a person typed
// them, so nothing can bring them back.
//
//   - -dump: the global `plexams` database -- rooms, study programs, NTAs,
//     permanent non-invigilators, the planner and the Anny config.
//
//   - -semester-dump: ONE semester database, of which exactly two collections are
//     read: semester_config_input and preplan_exams. Exams, teachers, student
//     registrations, conflicts and Anny bookings are deliberately NOT imported --
//     they come back by importing them from ZPA/Primuss/Anny. The report lists
//     what was left behind, so a collection nobody expected to be full is
//     visible instead of silently dropped.
//
//     go run ./tools/mongo2pg \
//     -dump          /workspace/semester/mongo-backup/plexams \
//     -semester-dump /workspace/semester/mongo-backup/2026-WS \
//     -uri "postgres://plexams@127.0.0.1:5433/plexams?sslmode=disable" \
//     -dry-run
//
// Writes go through the same typed db.PG methods the application uses, so the
// rows land exactly as the running code expects -- the reason this is a Go
// program and not a pile of hand-written INSERTs. It brings the schema up first,
// so the target may be a completely empty database.
//
// It is idempotent: every writer upserts, so a rehearsal can be repeated and a
// half-finished run can simply be run again.
package main

import (
	"context"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/obcode/plexams.go/db"
	"go.mongodb.org/mongo-driver/bson"
)

// options is what the operator asked for on the command line.
type options struct {
	globalDump   string
	semesterDump string
	semester     string
	uri          string
	dryRun       bool
}

func main() {
	var o options
	flag.StringVar(&o.globalDump, "dump", "", "directory holding the mongodump .bson files of the global `plexams` database")
	flag.StringVar(&o.semesterDump, "semester-dump", "", "directory holding the mongodump .bson files of ONE semester database")
	flag.StringVar(&o.semester, "semester", "", "the semester -semester-dump belongs to (default: the directory's name)")
	flag.StringVar(&o.uri, "uri", "", "PostgreSQL connection string")
	flag.BoolVar(&o.dryRun, "dry-run", false, "convert and report, but write nothing")
	flag.Parse()

	if (o.globalDump == "" && o.semesterDump == "") || o.uri == "" {
		fmt.Fprintln(os.Stderr,
			"usage: mongo2pg -uri <postgres-uri> [-dump <dir>] [-semester-dump <dir> [-semester <YYYY-SS>]] [-dry-run]")
		os.Exit(2)
	}

	if err := run(context.Background(), o); err != nil {
		fmt.Fprintf(os.Stderr, "\nFEHLER: %v\n", err)
		os.Exit(1)
	}
}

// report accumulates what the operator needs to see afterwards.
type report struct {
	imported map[string]int
	skipped  map[string]int
	notes    []note
	dropped  map[string]map[string]int // collection -> field -> count
	// jointPrograms are the joint study programs seen in the global dump. Only a
	// dry run uses them; a real one asks the database, which is the authority.
	jointPrograms []string
	// untouched lists the non-empty collections of the semester dump that are
	// deliberately not imported.
	untouched []string
}

func newReport() *report {
	return &report{
		imported: map[string]int{},
		skipped:  map[string]int{},
		dropped:  map[string]map[string]int{},
	}
}

func (r *report) note(ns ...note) { r.notes = append(r.notes, ns...) }

func (r *report) drop(collection string, fields []string) {
	if len(fields) == 0 {
		return
	}
	if r.dropped[collection] == nil {
		r.dropped[collection] = map[string]int{}
	}
	for _, f := range fields {
		r.dropped[collection][f]++
	}
}

func run(ctx context.Context, o options) error {
	semester, err := resolveSemester(o)
	if err != nil {
		return err
	}

	pg, err := db.NewPG(ctx, o.uri, semester)
	if err != nil {
		return fmt.Errorf("cannot reach postgres: %w", err)
	}
	defer pg.Close()

	rep := newReport()

	if o.dryRun {
		fmt.Println("== TROCKENLAUF -- es wird nichts geschrieben ==")
	} else {
		// The target may be an empty database: the point of the tool is to fill a
		// fresh one, and nothing else would have created the tables.
		if err := pg.MigrateSchema(ctx); err != nil {
			return fmt.Errorf("cannot migrate the schema: %w", err)
		}
	}

	if o.globalDump != "" {
		if err := importGlobal(ctx, pg, o, rep); err != nil {
			return err
		}
	}
	if o.semesterDump != "" {
		if err := importSemester(ctx, pg, o, semester, rep); err != nil {
			return err
		}
	}

	printReport(rep)
	return nil
}

// resolveSemester determines which semester -semester-dump belongs to. The
// directory name is the default because mongodump names it after the database,
// which was the semester.
//
// The format is checked here rather than left to the check constraint: a typo
// would otherwise surface as a raw SQLSTATE in the middle of an import, after the
// global master data has already been written.
func resolveSemester(o options) (string, error) {
	if o.semesterDump == "" {
		return "", nil
	}
	semester := o.semester
	if semester == "" {
		semester = filepath.Base(filepath.Clean(o.semesterDump))
	}
	if !db.IsSemester(semester) {
		return "", fmt.Errorf("%q is not a semester (expected YYYY-SS or YYYY-WS) -- say which one with -semester", semester)
	}
	return semester, nil
}

// importGlobal moves the global master data: the collections no import can
// recreate because they are hand-maintained.
func importGlobal(ctx context.Context, pg *db.PG, o options, rep *report) error {
	fmt.Printf("Globaler Dump: %s\n\n", o.globalDump)

	steps := []struct {
		file string
		fn   func(context.Context, *db.PG, *doc, bool, *report) error
	}{
		{"rooms", importRoom},
		{"study_programs", importStudyProgram},
		{"nta", importNta},
		{"permanent_non_invigilators", importNonInvigilator},
		{"anny_config", importAnnyConfig},
	}

	for _, s := range steps {
		docs, err := readBSON(filepath.Join(o.globalDump, s.file+".bson"))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				fmt.Printf("  %-28s nicht im Dump -- uebersprungen\n", s.file)
				continue
			}
			return fmt.Errorf("%s: %w", s.file, err)
		}
		for _, m := range docs {
			d := newDoc(m)
			if err := s.fn(ctx, pg, d, o.dryRun, rep); err != nil {
				return fmt.Errorf("%s: %w", s.file, err)
			}
			rep.drop(s.file, d.leftovers())
		}
		fmt.Printf("  %-28s %3d gelesen, %3d importiert, %3d uebersprungen\n",
			s.file, len(docs), rep.imported[s.file], rep.skipped[s.file])
	}

	// active_semester and email_templates are deliberately absent above:
	// active_semester rebuilds itself on the next start, and email_templates holds
	// only planner-authored overrides -- empty in the dump.

	return nil
}

func printReport(rep *report) {
	if len(rep.dropped) > 0 {
		fmt.Println("\n== Verworfene Felder (das Modell kennt sie nicht mehr) ==")
		colls := make([]string, 0, len(rep.dropped))
		for c := range rep.dropped {
			colls = append(colls, c)
		}
		sort.Strings(colls)
		for _, c := range colls {
			fields := make([]string, 0, len(rep.dropped[c]))
			for f := range rep.dropped[c] {
				fields = append(fields, f)
			}
			sort.Strings(fields)
			for _, f := range fields {
				fmt.Printf("  %-28s %-22s in %d Dokument(en)\n", c, f, rep.dropped[c][f])
			}
		}
	}

	if len(rep.untouched) > 0 {
		fmt.Println("\n== Nicht importiert (kommt aus ZPA/Primuss/Anny zurueck) ==")
		fmt.Println("   Steht hier etwas Handgepflegtes -- Constraints, verbundene Pruefungen, ein fertiger Plan --,")
		fmt.Println("   dann ist die Annahme des Cut-overs fuer dieses Semester falsch. Vorher klaeren.")
		for _, line := range rep.untouched {
			fmt.Printf("  %s\n", line)
		}
	}

	if len(rep.notes) > 0 {
		fmt.Println("\n== Anmerkungen (bitte nach dem Import in der GUI pruefen) ==")
		for _, n := range rep.notes {
			fmt.Printf("  [%s] %s\n", n.Kind, n.Message)
		}
	} else {
		fmt.Println("\nKeine Anmerkungen.")
	}
}

// ---- per-collection steps ------------------------------------------------------

func importRoom(ctx context.Context, pg *db.PG, d *doc, dry bool, rep *report) error {
	room, notes := convertRoom(d)
	rep.note(notes...)
	if room.Name == "" {
		rep.skipped["rooms"]++
		return nil
	}
	if !dry {
		// Add inserts, Replace updates -- neither upserts on its own, because the
		// application's semantics say a room either exists or does not. So decide by
		// looking: that keeps a repeated run a no-op instead of a duplicate-key error.
		// RoomByName reports "not found" as an error, by the same convention.
		if existing, err := pg.RoomByName(ctx, room.Name); err == nil && existing != nil {
			if _, err := pg.ReplaceRoom(ctx, room); err != nil {
				return fmt.Errorf("room %s: %w", room.Name, err)
			}
		} else if _, err := pg.AddRoom(ctx, room); err != nil {
			return fmt.Errorf("room %s: %w", room.Name, err)
		}
	}
	rep.imported["rooms"]++
	return nil
}

func importStudyProgram(ctx context.Context, pg *db.PG, d *doc, dry bool, rep *report) error {
	p, notes := convertStudyProgram(d)
	rep.note(notes...)
	if p.Shortname == "" {
		rep.skipped["study_programs"]++
		return nil
	}
	if !dry {
		if err := pg.UpsertStudyProgram(ctx, p); err != nil {
			return fmt.Errorf("study program %s: %w", p.Shortname, err)
		}
	}
	if p.Category == "joint" {
		rep.jointPrograms = append(rep.jointPrograms, p.Shortname)
	}
	rep.imported["study_programs"]++
	return nil
}

func importNta(ctx context.Context, pg *db.PG, d *doc, dry bool, rep *report) error {
	n, notes := convertNta(d)
	rep.note(notes...)
	if n.Mtknr == "" {
		rep.skipped["nta"]++
		return nil
	}
	if !dry {
		// Nta returns (nil, nil) when there is none -- the convention for this one.
		existing, err := pg.Nta(ctx, n.Mtknr)
		if err != nil {
			return fmt.Errorf("nta %s: %w", n.Mtknr, err)
		}
		if existing != nil {
			if _, err := pg.ReplaceNta(ctx, n); err != nil {
				return fmt.Errorf("nta %s: %w", n.Mtknr, err)
			}
		} else if _, err := pg.AddNta(ctx, n); err != nil {
			return fmt.Errorf("nta %s: %w", n.Mtknr, err)
		}
	}
	rep.imported["nta"]++
	return nil
}

func importNonInvigilator(ctx context.Context, pg *db.PG, d *doc, dry bool, rep *report) error {
	n, notes := convertNonInvigilator(d)
	rep.note(notes...)
	if n.TeacherID == 0 {
		rep.skipped["permanent_non_invigilators"]++
		return nil
	}
	if !dry {
		if err := pg.UpsertPermanentNonInvigilator(ctx, n); err != nil {
			return fmt.Errorf("non-invigilator %d: %w", n.TeacherID, err)
		}
	}
	rep.imported["permanent_non_invigilators"]++
	return nil
}

func importAnnyConfig(ctx context.Context, pg *db.PG, d *doc, dry bool, rep *report) error {
	c := convertAnnyConfig(d)
	if !dry {
		if err := pg.SetAnnyConfig(ctx, c); err != nil {
			return fmt.Errorf("anny config: %w", err)
		}
	}
	rep.imported["anny_config"]++
	return nil
}

// ---- reading a mongodump .bson file ---------------------------------------------

// readBSON reads a mongodump collection file: BSON documents back to back, each
// prefixed with its own little-endian int32 length (the length includes itself).
func readBSON(path string) ([]map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var out []map[string]any
	for i := 0; i < len(raw); {
		if i+4 > len(raw) {
			return nil, fmt.Errorf("truncated document header at byte %d", i)
		}
		size := int(binary.LittleEndian.Uint32(raw[i:]))
		if size < 5 || i+size > len(raw) {
			return nil, fmt.Errorf("implausible document length %d at byte %d", size, i)
		}
		var m map[string]any
		if err := bson.Unmarshal(raw[i:i+size], &m); err != nil {
			return nil, fmt.Errorf("cannot decode document at byte %d: %w", i, err)
		}
		out = append(out, m)
		i += size
	}
	return out, nil
}
