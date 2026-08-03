package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGeneratedCodeUsesTimeTime guards the timestamptz overrides in sqlc.yaml.
//
// With the pgx/v5 driver sqlc maps timestamptz to pgtype.Timestamptz unless told
// otherwise, and emit_pointers_for_null_types does not reach it. The override is
// easy to lose (its db_type must be spelled "timestamptz" -- the qualified
// "pg_catalog.timestamptz" matches nothing and fails silently), and losing it
// would be quiet: the code would still compile, every mapper would just need an
// unwrapping step, and each of those is a place to drop the Europe/Berlin
// location that makes a time.Time usable as a map key.
//
// See TestTimestamptzKeepsLocation for why the location is the thing that matters.
func TestGeneratedCodeUsesTimeTime(t *testing.T) {
	entries, err := os.ReadDir("sqlc")
	if err != nil {
		t.Fatalf("cannot read the generated code: %v", err)
	}

	found := false
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		found = true

		path := filepath.Join("sqlc", entry.Name())
		source, err := os.ReadFile(path) //nolint:gosec // a fixed path inside the package
		if err != nil {
			t.Fatalf("cannot read %s: %v", path, err)
		}
		if strings.Contains(string(source), "pgtype.Timestamptz") {
			t.Errorf("%s uses pgtype.Timestamptz -- the timestamptz overrides in sqlc.yaml are gone", path)
		}
	}

	if !found {
		t.Fatal("no generated code found in db/sqlc")
	}
}
