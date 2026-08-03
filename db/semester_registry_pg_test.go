package db_test

import (
	"testing"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
)

// TestEnsureMetaDoesNotInventALabel pins the rule EnsureMeta always had: it
// stamps the schema version but must not write a semester label, because a
// derived or guessed one would then look authoritative.
func TestEnsureMetaDoesNotInventALabel(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	if err := pg.EnsureMeta(ctx, 2); err != nil {
		t.Fatalf("EnsureMeta: %v", err)
	}

	meta, err := pg.GetSemesterMeta(ctx)
	if err != nil {
		t.Fatalf("GetSemesterMeta: %v", err)
	}
	if meta == nil {
		t.Fatal("GetSemesterMeta = nil after EnsureMeta")
	}
	if meta.SchemaVersion != 2 {
		t.Errorf("SchemaVersion = %d, want 2", meta.SchemaVersion)
	}
	if meta.Semester != "" {
		t.Errorf("Semester = %q, want empty -- EnsureMeta must not persist a label", meta.Semester)
	}
	if meta.ReadOnly {
		t.Error("a fresh workspace is read-only")
	}

	// Calling it again must not reset an explicit label or the version.
	if err := pg.SetMetaSemester(ctx, "2026 WS", 2); err != nil {
		t.Fatalf("SetMetaSemester: %v", err)
	}
	if err := pg.EnsureMeta(ctx, 2); err != nil {
		t.Fatalf("EnsureMeta again: %v", err)
	}
	meta, err = pg.GetSemesterMeta(ctx)
	if err != nil {
		t.Fatalf("GetSemesterMeta: %v", err)
	}
	if meta.Semester != "2026 WS" {
		t.Errorf("Semester = %q, want the explicit label to survive", meta.Semester)
	}
}

// A workspace that does not exist has no meta -- (nil, nil), which is how
// Migrate and AllSemesterNames tell "not a workspace" from "a broken one".
func TestMetaOfAnUnknownWorkspaceIsNil(t *testing.T) {
	pg := pgtest.NewDB(t)

	meta, err := pg.GetSemesterMeta(t.Context())
	if err != nil {
		t.Fatalf("GetSemesterMeta: %v", err)
	}
	if meta != nil {
		t.Errorf("GetSemesterMeta = %#v, want nil", meta)
	}
}

func TestSemesterMetaFlags(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	if err := pg.EnsureMeta(ctx, 2); err != nil {
		t.Fatalf("EnsureMeta: %v", err)
	}
	if err := pg.SetSemesterReadOnly(ctx, true); err != nil {
		t.Fatalf("SetSemesterReadOnly: %v", err)
	}
	at := berlin(t, "2027-01-20 08:30")
	if err := pg.SetLastDumpAt(ctx, at); err != nil {
		t.Fatalf("SetLastDumpAt: %v", err)
	}

	meta, err := pg.GetSemesterMeta(ctx)
	if err != nil {
		t.Fatalf("GetSemesterMeta: %v", err)
	}
	if !meta.ReadOnly {
		t.Error("ReadOnly was not stored")
	}
	if meta.LastDumpAt == nil || !meta.LastDumpAt.Equal(at) {
		t.Errorf("LastDumpAt = %v, want %v", meta.LastDumpAt, at)
	}
	// Setting one flag must not clear the other -- these were separate $set calls.
	if meta.SchemaVersion != 2 {
		t.Errorf("SchemaVersion = %d, want 2", meta.SchemaVersion)
	}
}

// SwitchTo is where "one database per semester" becomes "one column". The
// override wins over the stored label and is deliberately not persisted.
func TestSwitchToResolvesTheLogicalSemester(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	exec(t, pg, `insert into semester (id, semester, schema_version)
	             values ('2026-WS', '2026 WS', 2), ('Test26SS-v2', '2026 SS', 2),
	                    ('nolabel', '', 2)`)

	if got := pg.SwitchTo(ctx, "Test26SS-v2", ""); got != "2026 SS" {
		t.Errorf("SwitchTo without override = %q, want the stored label", got)
	}
	if got := pg.DatabaseName(); got != "Test26SS-v2" {
		t.Errorf("DatabaseName = %q, want the workspace id", got)
	}

	// An override wins -- that is the whole point of a clone.
	if got := pg.SwitchTo(ctx, "Test26SS-v2", "2027 SS"); got != "2027 SS" {
		t.Errorf("SwitchTo with override = %q, want the override", got)
	}
	// ...and is not written back.
	meta, err := pg.GetSemesterMeta(ctx)
	if err != nil {
		t.Fatalf("GetSemesterMeta: %v", err)
	}
	if meta.Semester != "2026 SS" {
		t.Errorf("the override was persisted: stored label is now %q", meta.Semester)
	}

	// A workspace without a stored label falls back to its id.
	if got := pg.SwitchTo(ctx, "nolabel", ""); got != "nolabel" {
		t.Errorf("SwitchTo = %q, want the id derived label", got)
	}
	if got := pg.SemesterForDatabase(ctx, "2026-WS"); got != "2026 WS" {
		t.Errorf("SemesterForDatabase = %q, want the stored label", got)
	}
	// The id-to-label derivation replaces the first dash with a space.
	if got := pg.SemesterForDatabase(ctx, "2025-SS"); got != "2025 SS" {
		t.Errorf("SemesterForDatabase of an unknown workspace = %q, want 2025 SS", got)
	}
}

// AllSemesterNames answers in one query what Mongo needed a ListDatabaseNames
// plus two probes per database for. `Compatible` means "carries a config".
func TestAllSemesterNames(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	exec(t, pg, `insert into semester (id, semester, schema_version, read_only)
	             values ('2026-WS', '2026 WS', 2, false),
	                    ('2026-SS', '2026 SS', 2, true),
	                    ('Test26SS-v2', '2026 SS', 2, false)`)

	// Only two of them get a config.
	for _, id := range []string{"2026-WS", "Test26SS-v2"} {
		if err := pg.SaveSemesterConfigInputToDatabase(ctx, id,
			&model.SemesterConfigInput{}); err != nil {
			t.Fatalf("SaveSemesterConfigInputToDatabase(%s): %v", id, err)
		}
	}

	semesters, err := pg.AllSemesterNames(ctx)
	if err != nil {
		t.Fatalf("AllSemesterNames: %v", err)
	}
	if len(semesters) != 3 {
		t.Fatalf("got %d workspaces, want 3", len(semesters))
	}

	// Newest logical semester first, then the id -- so the canonical workspace
	// comes before a test clone of the same semester.
	if semesters[0].ID != "2026-WS" {
		t.Errorf("first = %s, want 2026-WS (newest)", semesters[0].ID)
	}
	if semesters[1].ID != "2026-SS" || semesters[2].ID != "Test26SS-v2" {
		t.Errorf("order = %s, %s; want 2026-SS before Test26SS-v2",
			semesters[1].ID, semesters[2].ID)
	}

	byID := map[string]*model.Semester{}
	for _, s := range semesters {
		byID[s.ID] = s
	}
	if !byID["2026-WS"].Compatible {
		t.Error("2026-WS has a config but is not compatible")
	}
	if byID["2026-SS"].Compatible {
		t.Error("2026-SS has no config but is compatible")
	}
	if !byID["2026-SS"].ReadOnly {
		t.Error("the read-only flag was lost")
	}
	if byID["2026-WS"].Semester == nil || *byID["2026-WS"].Semester != "2026 WS" {
		t.Errorf("Semester = %v, want the stored label", byID["2026-WS"].Semester)
	}
}

// A config for a workspace that does not exist is rejected. Under MongoDB the
// insert created the database as a side effect, so a typo produced a second,
// empty workspace that then appeared in the switcher.
func TestAConfigNeedsItsWorkspace(t *testing.T) {
	pg := pgtest.NewDB(t)

	if err := pg.SaveSemesterConfigInputToDatabase(t.Context(), "tpyo-2026",
		&model.SemesterConfigInput{}); err == nil {
		t.Error("a config for a workspace that does not exist was accepted")
	}
}

func TestSemesterConfigInputRoundTrip(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	exec(t, pg, `insert into semester (id, semester, schema_version)
	             values ('2026-WS', '2026 WS', 2)`)

	if config, err := pg.GetSemesterConfigInput(ctx); err != nil {
		t.Fatalf("GetSemesterConfigInput: %v", err)
	} else if config != nil {
		t.Errorf("GetSemesterConfigInput = %#v, want nil before anything is stored", config)
	}
	if pg.DatabaseHasConfig(ctx, "2026-WS") {
		t.Error("DatabaseHasConfig = true before a config was stored")
	}

	from, until := berlin(t, "2027-02-01 00:00"), berlin(t, "2027-02-20 00:00")
	input := &model.SemesterConfigInput{From: from, Until: until}
	if err := pg.SaveSemesterConfigInput(ctx, input); err != nil {
		t.Fatalf("SaveSemesterConfigInput: %v", err)
	}

	got, err := pg.GetSemesterConfigInput(ctx)
	if err != nil {
		t.Fatalf("GetSemesterConfigInput: %v", err)
	}
	if got == nil || !got.From.Equal(from) {
		t.Errorf("config = %#v, want the stored one", got)
	}
	if !pg.DatabaseHasConfig(ctx, "2026-WS") {
		t.Error("DatabaseHasConfig = false after a config was stored")
	}

	// Saving again replaces rather than accumulating -- it was a drop and insert.
	input.From = from.AddDate(0, 0, 1)
	if err := pg.SaveSemesterConfigInput(ctx, input); err != nil {
		t.Fatalf("SaveSemesterConfigInput: %v", err)
	}
	if n := count(t, pg, `select count(*) from semester_config_input`); n != 1 {
		t.Errorf("semester_config_input rows = %d, want 1", n)
	}
	got, err = pg.GetSemesterConfigInput(ctx)
	if err != nil {
		t.Fatalf("GetSemesterConfigInput: %v", err)
	}
	if !got.From.Equal(from.AddDate(0, 0, 1)) {
		t.Errorf("From = %v, want the second write", got.From)
	}
}

func TestResolveStartSemester(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	// Nothing at all.
	if _, _, ok := pg.ResolveStartSemester(ctx); ok {
		t.Error("ResolveStartSemester found something in an empty database")
	}

	exec(t, pg, `insert into semester (id, semester, schema_version)
	             values ('2026-WS', '2026 WS', 2), ('2026-SS', '2026 SS', 2)`)
	if err := pg.SaveSemesterConfigInputToDatabase(ctx, "2026-SS",
		&model.SemesterConfigInput{}); err != nil {
		t.Fatalf("SaveSemesterConfigInputToDatabase: %v", err)
	}

	// No active semester: the newest compatible one wins -- 2026-WS is newer but
	// has no config, so 2026-SS it is.
	semester, database, ok := pg.ResolveStartSemester(ctx)
	if !ok || database != "2026-SS" || semester != "2026 SS" {
		t.Errorf("ResolveStartSemester = %q, %q, %v; want the newest compatible one",
			semester, database, ok)
	}

	// An active semester wins, as long as it still has a config.
	if err := pg.SaveSemesterConfigInputToDatabase(ctx, "2026-WS",
		&model.SemesterConfigInput{}); err != nil {
		t.Fatalf("SaveSemesterConfigInputToDatabase: %v", err)
	}
	pg.SwitchTo(ctx, "2026-WS", "")
	if err := pg.SaveActiveSemester(ctx); err != nil {
		t.Fatalf("SaveActiveSemester: %v", err)
	}
	semester, database, ok = pg.ResolveStartSemester(ctx)
	if !ok || database != "2026-WS" || semester != "2026 WS" {
		t.Errorf("ResolveStartSemester = %q, %q, %v; want the active one",
			semester, database, ok)
	}
}

// Migrate no longer migrates -- there are no pre-cut-over data shapes in
// PostgreSQL. It is the version guard: a workspace written by a newer binary is
// left alone rather than downgraded.
func TestMigrateIsTheVersionGuard(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	// No workspace: nothing to do, no error.
	if err := pg.Migrate(ctx); err != nil {
		t.Fatalf("Migrate on nothing: %v", err)
	}

	exec(t, pg, `insert into semester (id, semester, schema_version)
	             values ('2026-WS', '2026 WS', 1)`)
	if err := pg.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	meta, err := pg.GetSemesterMeta(ctx)
	if err != nil {
		t.Fatalf("GetSemesterMeta: %v", err)
	}
	if meta.SchemaVersion != db.CurrentSchemaVersion {
		t.Errorf("SchemaVersion = %d, want it stamped to %d",
			meta.SchemaVersion, db.CurrentSchemaVersion)
	}

	// A workspace from the future is left exactly as it is.
	exec(t, pg, `update semester set schema_version = $1 where id = '2026-WS'`,
		db.CurrentSchemaVersion+1)
	if err := pg.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	meta, err = pg.GetSemesterMeta(ctx)
	if err != nil {
		t.Fatalf("GetSemesterMeta: %v", err)
	}
	if meta.SchemaVersion != db.CurrentSchemaVersion+1 {
		t.Errorf("SchemaVersion = %d, want the newer version untouched", meta.SchemaVersion)
	}

	// A read-only workspace is not stamped either -- migrating it would defeat
	// the protection.
	exec(t, pg, `update semester set schema_version = 1, read_only = true where id = '2026-WS'`)
	if err := pg.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	meta, err = pg.GetSemesterMeta(ctx)
	if err != nil {
		t.Fatalf("GetSemesterMeta: %v", err)
	}
	if meta.SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d, want a read-only workspace left alone", meta.SchemaVersion)
	}
}
