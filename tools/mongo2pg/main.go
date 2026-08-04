// Command mongo2pg imports the global MongoDB master data into PostgreSQL.
//
// It is a ONE-OFF for the cut-over and is meant to be deleted afterwards. It
// lives in its own module so that go.mongodb.org/mongo-driver does not return to
// the main one, which the migration just finished removing.
//
// It reads the ARCHIVED DUMP (mongodump's .bson files), not a live server. That
// makes a rehearsal reproducible and possible without VPN or a running mongod,
// and the dump is the artifact the migration plan keeps anyway.
//
// Only the global database matters. Semesters are not migrated: they are
// archived as Mongo dumps and re-imported from ZPA/Primuss/Anny.
//
//	go run ./tools/mongo2pg \
//	    -dump /workspace/semester/mongo-backup/plexams \
//	    -uri "postgres://plexams@127.0.0.1:5433/plexams?sslmode=disable" \
//	    -dry-run
//
// Writes go through the same typed db.PG methods the application uses, so the
// rows land exactly as the running code expects -- the reason this is a Go
// program and not a pile of hand-written INSERTs.
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

func main() {
	dumpDir := flag.String("dump", "", "directory holding the mongodump .bson files of the global `plexams` database")
	uri := flag.String("uri", "", "PostgreSQL connection string")
	dryRun := flag.Bool("dry-run", false, "convert and report, but write nothing")
	flag.Parse()

	if *dumpDir == "" || *uri == "" {
		fmt.Fprintln(os.Stderr, "usage: mongo2pg -dump <dir> -uri <postgres-uri> [-dry-run]")
		os.Exit(2)
	}

	if err := run(context.Background(), *dumpDir, *uri, *dryRun); err != nil {
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

func run(ctx context.Context, dumpDir, uri string, dryRun bool) error {
	// The workspace is irrelevant: everything here is global master data, and none
	// of the writers involved reads semesterID.
	pg, err := db.NewPG(ctx, uri, "")
	if err != nil {
		return fmt.Errorf("cannot reach postgres: %w", err)
	}
	defer pg.Close()

	rep := newReport()

	if dryRun {
		fmt.Println("== TROCKENLAUF -- es wird nichts geschrieben ==")
	}
	fmt.Printf("Dump: %s\n\n", dumpDir)

	steps := []struct {
		file string
		fn   func(context.Context, *db.PG, *doc, bool, *report) error
	}{
		{"rooms", importRoom},
		{"study_programs", importStudyProgram},
		{"nta", importNta},
		{"permanent_non_invigilators", importNonInvigilator},
		{"planer", importPlaner},
		{"anny_config", importAnnyConfig},
	}

	for _, s := range steps {
		docs, err := readBSON(filepath.Join(dumpDir, s.file+".bson"))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				fmt.Printf("  %-28s nicht im Dump -- uebersprungen\n", s.file)
				continue
			}
			return fmt.Errorf("%s: %w", s.file, err)
		}
		for _, m := range docs {
			d := newDoc(m)
			if err := s.fn(ctx, pg, d, dryRun, rep); err != nil {
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

	printReport(rep)
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

func importPlaner(ctx context.Context, pg *db.PG, d *doc, dry bool, rep *report) error {
	p := convertPlaner(d)
	if !dry {
		if err := pg.SavePlaner(ctx, p); err != nil {
			return fmt.Errorf("planer: %w", err)
		}
	}
	rep.imported["planer"]++
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
