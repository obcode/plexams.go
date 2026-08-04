package db_test

import (
	"testing"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
)

// EnsureMeta registers the current semester and stamps a fresh row's schema
// version. It must not move the version of a semester that already has one --
// only Migrate does that.
func TestEnsureMetaStampsOnlyAFreshSemester(t *testing.T) {
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
	if meta.ReadOnly {
		t.Error("a fresh semester is read-only")
	}

	if err := pg.EnsureMeta(ctx, 3); err != nil {
		t.Fatalf("EnsureMeta again: %v", err)
	}
	meta, err = pg.GetSemesterMeta(ctx)
	if err != nil {
		t.Fatalf("GetSemesterMeta: %v", err)
	}
	if meta.SchemaVersion != 2 {
		t.Errorf("SchemaVersion = %d, want the stamped 2 to survive", meta.SchemaVersion)
	}
}

// EnsureSemester registers a semester other than the current one -- what
// createSemester does, and what the three SetMetaSemester* variants collapsed
// into once the label stopped being stored.
func TestEnsureSemesterRegistersAnotherSemester(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	if err := pg.EnsureSemester(ctx, "2027-SS", 2); err != nil {
		t.Fatalf("EnsureSemester: %v", err)
	}
	if n := count(t, pg, `select count(*) from semester where id = '2027-SS'`); n != 1 {
		t.Errorf("rows for 2027-SS = %d, want 1", n)
	}
	// ...and the current one is untouched by it.
	meta, err := pg.GetSemesterMeta(ctx)
	if err != nil {
		t.Fatalf("GetSemesterMeta: %v", err)
	}
	if meta != nil {
		t.Errorf("GetSemesterMeta = %#v, want nil -- 2026-WS was never registered", meta)
	}
}

// The id format is a constraint, not a convention: the logical semester ZPA sees
// is derived from it, so a name it cannot be derived from must not get in.
//
// db.IsSemester is the Go copy of that constraint, so the two are asserted
// together -- a drift between them would show up as a raw SQLSTATE in the GUI
// instead of a message naming the bad value.
func TestSemesterIDFormatIsEnforced(t *testing.T) {
	pg := pgtest.NewDB(t)

	cases := map[string]bool{
		"2026-WS":      true,
		"2026-SS":      true,
		"Test26SS-v2":  false, // the clone name this whole change removes
		"2026-XX":      false,
		"2026 SS":      false, // the logical form, not the id
		"":             false,
		"2026-SS-Test": false,
	}
	for id, want := range cases {
		if got := db.IsSemester(id); got != want {
			t.Errorf("IsSemester(%q) = %v, want %v", id, got, want)
		}
		err := pg.EnsureSemester(t.Context(), id, 2)
		if want && err != nil {
			t.Errorf("EnsureSemester(%q): %v", id, err)
		}
		if !want && err == nil {
			t.Errorf("EnsureSemester(%q) was accepted", id)
		}
	}
}

// A semester that is not registered has no meta -- (nil, nil), which is how
// Migrate and AllSemesterNames tell "not a semester" from "a broken one".
func TestMetaOfAnUnregisteredSemesterIsNil(t *testing.T) {
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

// SwitchTo is where "one database per semester" became "one column". The logical
// semester it returns is derived, not looked up -- the stored label and the
// override for test clones are both gone.
func TestSwitchToDerivesTheLogicalSemester(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	if got := pg.SwitchTo(ctx, "2026-SS"); got != "2026 SS" {
		t.Errorf("SwitchTo = %q, want the derived label", got)
	}
	if got := pg.Semester(); got != "2026-SS" {
		t.Errorf("Semester = %q, want the id", got)
	}
	// It does not read anything, so an unregistered semester is not an error here
	// -- switching to one that has no config yet is how a new semester is set up.
	if got := pg.SwitchTo(ctx, "2027-WS"); got != "2027 WS" {
		t.Errorf("SwitchTo = %q, want 2027 WS", got)
	}
}

// AllSemesterNames answers in one query what Mongo needed a ListDatabaseNames
// plus two probes per database for. `Compatible` means "carries a config".
func TestAllSemesterNames(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	exec(t, pg, `insert into semester (id, schema_version, read_only)
	             values ('2026-WS', 2, false),
	                    ('2026-SS', 2, true),
	                    ('2025-WS', 2, false)`)

	// Only two of them get a config.
	for _, id := range []string{"2026-WS", "2025-WS"} {
		if err := pg.SaveSemesterConfigInputFor(ctx, id,
			&model.SemesterConfigInput{}); err != nil {
			t.Fatalf("SaveSemesterConfigInputFor(%s): %v", id, err)
		}
	}

	semesters, err := pg.AllSemesterNames(ctx)
	if err != nil {
		t.Fatalf("AllSemesterNames: %v", err)
	}
	if len(semesters) != 3 {
		t.Fatalf("got %d semesters, want 3", len(semesters))
	}

	// Newest first. The id sorts like the label it is derived from, so WS comes
	// before the SS of the same year.
	for i, want := range []string{"2026-WS", "2026-SS", "2025-WS"} {
		if semesters[i].ID != want {
			t.Errorf("semesters[%d] = %s, want %s", i, semesters[i].ID, want)
		}
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
}

// A config for a semester that is not registered is rejected. Under MongoDB the
// insert created the database as a side effect, so a typo produced a second,
// empty database that then appeared in the switcher.
func TestAConfigNeedsItsSemester(t *testing.T) {
	pg := pgtest.NewDB(t)

	if err := pg.SaveSemesterConfigInputFor(t.Context(), "2027-SS",
		&model.SemesterConfigInput{}); err == nil {
		t.Error("a config for a semester that is not registered was accepted")
	}
}

func TestSemesterConfigInputRoundTrip(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	exec(t, pg, `insert into semester (id, schema_version) values ('2026-WS', 2)`)

	if config, err := pg.GetSemesterConfigInput(ctx); err != nil {
		t.Fatalf("GetSemesterConfigInput: %v", err)
	} else if config != nil {
		t.Errorf("GetSemesterConfigInput = %#v, want nil before anything is stored", config)
	}
	if pg.SemesterHasConfig(ctx, "2026-WS") {
		t.Error("SemesterHasConfig = true before a config was stored")
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
	if !pg.SemesterHasConfig(ctx, "2026-WS") {
		t.Error("SemesterHasConfig = false after a config was stored")
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
	if _, ok := pg.ResolveStartSemester(ctx); ok {
		t.Error("ResolveStartSemester found something in an empty database")
	}

	exec(t, pg, `insert into semester (id, schema_version)
	             values ('2026-WS', 2), ('2026-SS', 2)`)
	if err := pg.SaveSemesterConfigInputFor(ctx, "2026-SS",
		&model.SemesterConfigInput{}); err != nil {
		t.Fatalf("SaveSemesterConfigInputFor: %v", err)
	}

	// No active semester: the newest compatible one wins -- 2026-WS is newer but
	// has no config, so 2026-SS it is.
	semester, ok := pg.ResolveStartSemester(ctx)
	if !ok || semester != "2026-SS" {
		t.Errorf("ResolveStartSemester = %q, %v; want the newest compatible one", semester, ok)
	}

	// An active semester wins, as long as it still has a config.
	if err := pg.SaveSemesterConfigInputFor(ctx, "2026-WS",
		&model.SemesterConfigInput{}); err != nil {
		t.Fatalf("SaveSemesterConfigInputFor: %v", err)
	}
	pg.SwitchTo(ctx, "2026-WS")
	if err := pg.SaveActiveSemester(ctx); err != nil {
		t.Fatalf("SaveActiveSemester: %v", err)
	}
	semester, ok = pg.ResolveStartSemester(ctx)
	if !ok || semester != "2026-WS" {
		t.Errorf("ResolveStartSemester = %q, %v; want the active one", semester, ok)
	}
}

// Migrate no longer migrates -- there are no pre-cut-over data shapes in
// PostgreSQL. It is the version guard: data written by a newer binary is left
// alone rather than downgraded.
func TestMigrateIsTheVersionGuard(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	// Not registered: nothing to do, no error.
	if err := pg.Migrate(ctx); err != nil {
		t.Fatalf("Migrate on nothing: %v", err)
	}

	exec(t, pg, `insert into semester (id, schema_version) values ('2026-WS', 1)`)
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

	// Data from the future is left exactly as it is.
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

	// A read-only semester is not stamped either -- migrating it would defeat
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
		t.Errorf("SchemaVersion = %d, want a read-only semester left alone", meta.SchemaVersion)
	}
}
