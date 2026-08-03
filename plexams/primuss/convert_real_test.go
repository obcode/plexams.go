package primuss

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestConvertRealSammellisten runs the four converters over a real Sammellisten
// directory and fails on any conversion problem.
//
// It is skipped unless PLEXAMS_TEST_SAMMELLISTEN points at one, because those
// files contain Matrikelnummern and names and must not live in this (public)
// repository. Point it at a semester in the `semester` repo:
//
//	PLEXAMS_TEST_SAMMELLISTEN=/workspace/semester/past/2026-SS/Sammellisten \
//	    go test ./plexams/primuss/ -run RealSammellisten -v
//
// This is the check that the header mapping matches what Primuss actually
// delivers -- the structs alone cannot tell you that `Sum.` has a dot, that the
// planning file says `AnCo` and not `AnCode`, or that the examer is `pruefer` in
// one file and `Prüfer` in the next.
func TestConvertRealSammellisten(t *testing.T) {
	root := os.Getenv("PLEXAMS_TEST_SAMMELLISTEN")
	if root == "" {
		t.Skip("PLEXAMS_TEST_SAMMELLISTEN not set")
	}

	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.EqualFold(filepath.Ext(path), ".xlsx") {
			return err
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if len(files) == 0 {
		t.Fatalf("no .xlsx below %s", root)
	}

	// program -> kind -> ancodes of the exam catalogue, to check the conflicts
	// against the same catalogue the importer would.
	catalogue := make(map[string]map[int]bool)
	totals := map[string]int{}

	for _, path := range files {
		program, kind := detectPrimussFile(filepath.Base(path))
		if kind != "exams" {
			continue
		}
		data, err := os.ReadFile(path) //nolint:gosec // a path the operator passed in
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		known, err := knownAncodes(data)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		catalogue[program] = known
	}

	for _, path := range files {
		base := filepath.Base(path)
		program, kind := detectPrimussFile(base)
		if kind == "" {
			continue
		}
		data, err := os.ReadFile(path) //nolint:gosec // a path the operator passed in
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		sh, err := parseSheet(data)
		if err != nil {
			t.Fatalf("%s: %v", base, err)
		}

		var (
			rows  int
			probs []ConvertError
		)
		switch kind {
		case "studentregs":
			converted, errs := convertStudentRegs(sh)
			rows, probs = len(converted), errs
			withAaspf := 0
			for _, r := range converted {
				if r.Aaspf != nil {
					withAaspf++
				}
			}
			if len(converted) > 0 && withAaspf == 0 {
				t.Errorf("%s: no registration carries an AASPF -- the column moved or is unparsed", base)
			}
		case "exams":
			converted, errs := convertExams(sh)
			rows, probs = len(converted), errs
		case "count":
			converted, errs := convertCounts(sh)
			rows, probs = len(converted), errs
			withTotal := 0
			for _, r := range converted {
				if r.Total > 0 {
					withTotal++
				}
			}
			if len(converted) > 0 && withTotal == 0 {
				t.Errorf("%s: every total is 0 -- the %q column moved", base, colSum)
			}
		case "conflicts":
			converted, errs := convertConflicts(sh)
			if known, ok := catalogue[program]; ok {
				var dropped []ConvertError
				converted, dropped = dropConflictsWithUnknownExams(converted, known)
				errs = append(errs, dropped...)
			}
			rows, probs = len(converted), errs
		}

		totals[kind] += rows
		for _, p := range probs {
			t.Errorf("%s: %s", base, p)
		}
		if rows == 0 {
			t.Errorf("%s: converted to no rows at all", base)
		}
	}

	t.Logf("converted: %d exams, %d registrations, %d counts, %d conflict pairs",
		totals["exams"], totals["studentregs"], totals["count"], totals["conflicts"])
}
