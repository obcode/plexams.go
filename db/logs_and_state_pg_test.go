package db_test

import (
	"testing"
	"time"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
)

func logEntry(name string, at time.Time, args ...*model.MutationLogArg) *model.MutationLogEntry {
	return &model.MutationLogEntry{
		Time: at, Name: name, Type: "mutation", Args: args,
	}
}

func arg(key, value string) *model.MutationLogArg {
	return &model.MutationLogArg{Key: key, Value: value}
}

// TestMutationLogFilters covers the whole optional-filter surface at once,
// including the argument filter that was Mongo's single $elemMatch.
func TestMutationLogFilters(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()
	seedSemester(t, pg)

	early := berlin(t, "2027-01-20 08:30")
	late := berlin(t, "2027-01-21 10:00")
	user := "braun"

	first := logEntry("addPreplanExam", early, arg("ancode", "100"))
	first.Ancodes = []int{100}
	first.User = &user
	second := logEntry("setExamTime", late, arg("ancode", "200"))
	second.Ancodes = []int{200}

	for _, entry := range []*model.MutationLogEntry{first, second} {
		if err := pg.AddMutationLogEntry(ctx, entry); err != nil {
			t.Fatalf("AddMutationLogEntry: %v", err)
		}
	}

	all, err := pg.MutationLog(ctx, nil, nil, nil, nil, nil, nil, nil, 0)
	if err != nil {
		t.Fatalf("MutationLog: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("got %d entries, want 2", len(all))
	}
	// Newest first.
	if all[0].Name != "setExamTime" {
		t.Errorf("first entry = %s, want the newest", all[0].Name)
	}
	if len(all[0].Args) != 1 || all[0].Args[0].Key != "ancode" {
		t.Errorf("Args = %#v, want the stored pair back", all[0].Args)
	}

	name := "addPreplanExam"
	byName, err := pg.MutationLog(ctx, nil, &name, nil, nil, nil, nil, nil, 0)
	if err != nil {
		t.Fatalf("MutationLog(name): %v", err)
	}
	if len(byName) != 1 || byName[0].Name != name {
		t.Errorf("filter by name = %#v", byName)
	}

	ancode := 200
	byAncode, err := pg.MutationLog(ctx, nil, nil, &ancode, nil, nil, nil, nil, 0)
	if err != nil {
		t.Fatalf("MutationLog(ancode): %v", err)
	}
	if len(byAncode) != 1 || byAncode[0].Name != "setExamTime" {
		t.Errorf("filter by ancode = %#v", byAncode)
	}

	byUser, err := pg.MutationLog(ctx, nil, nil, nil, nil, &user, nil, nil, 0)
	if err != nil {
		t.Fatalf("MutationLog(user): %v", err)
	}
	if len(byUser) != 1 {
		t.Errorf("filter by user = %#v", byUser)
	}

	byArg, err := pg.MutationLog(ctx, nil, nil, nil,
		[]*model.ArgFilterInput{{Key: "ancode", Value: "100"}}, nil, nil, nil, 0)
	if err != nil {
		t.Fatalf("MutationLog(args): %v", err)
	}
	if len(byArg) != 1 || byArg[0].Name != "addPreplanExam" {
		t.Errorf("filter by argument = %#v", byArg)
	}

	since := late.Add(-time.Minute)
	bySince, err := pg.MutationLog(ctx, nil, nil, nil, nil, nil, &since, nil, 0)
	if err != nil {
		t.Fatalf("MutationLog(since): %v", err)
	}
	if len(bySince) != 1 || bySince[0].Name != "setExamTime" {
		t.Errorf("filter by since = %#v", bySince)
	}

	limited, err := pg.MutationLog(ctx, nil, nil, nil, nil, nil, nil, nil, 1)
	if err != nil {
		t.Fatalf("MutationLog(limit): %v", err)
	}
	if len(limited) != 1 {
		t.Errorf("limit 1 returned %d entries", len(limited))
	}

	// An empty string is not a filter -- every one of these treated "" as absent.
	empty := ""
	unfiltered, err := pg.MutationLog(ctx, &empty, &empty, nil, nil, &empty, nil, nil, 0)
	if err != nil {
		t.Fatalf("MutationLog(empty): %v", err)
	}
	if len(unfiltered) != 2 {
		t.Errorf("an empty filter string removed %d entries", 2-len(unfiltered))
	}

	latest, err := pg.LatestMutationTime(ctx)
	if err != nil {
		t.Fatalf("LatestMutationTime: %v", err)
	}
	if latest == nil || !latest.Equal(late) {
		t.Errorf("LatestMutationTime = %v, want %v", latest, late)
	}

	names, err := pg.MutationLogNames(ctx)
	if err != nil {
		t.Fatalf("MutationLogNames: %v", err)
	}
	if len(names) != 2 || names[0] != "addPreplanExam" {
		t.Errorf("MutationLogNames = %v", names)
	}
}

// An empty log has no latest time -- (nil, nil), not an error: BackupStatus
// reads it as "nothing changed yet".
func TestLatestMutationTimeOfAnEmptyLog(t *testing.T) {
	pg := pgtest.NewDB(t)
	seedSemester(t, pg)

	latest, err := pg.LatestMutationTime(t.Context())
	if err != nil {
		t.Fatalf("LatestMutationTime: %v", err)
	}
	if latest != nil {
		t.Errorf("LatestMutationTime = %v, want nil", latest)
	}
}

func TestSyncLog(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()
	seedSemester(t, pg)

	early := berlin(t, "2027-01-20 08:30")
	late := berlin(t, "2027-01-21 10:00")

	for _, entry := range []*model.SyncLogEntry{
		{Time: early, Operation: "zpa-import-exams", Direction: "import", System: "ZPA", OK: true,
			Added: 3, Entries: []*model.SyncChangeEntry{{Type: "added", Name: "100"}}},
		{Time: late, Operation: "zpa-upload-rooms", Direction: "upload", System: "ZPA", OK: true},
	} {
		if err := pg.AddSyncLogEntry(ctx, entry); err != nil {
			t.Fatalf("AddSyncLogEntry: %v", err)
		}
	}

	entries, err := pg.SyncLog(ctx, 0)
	if err != nil {
		t.Fatalf("SyncLog: %v", err)
	}
	if len(entries) != 2 || entries[0].Operation != "zpa-upload-rooms" {
		t.Fatalf("SyncLog = %#v, want both, newest first", entries)
	}
	if len(entries[1].Entries) != 1 || entries[1].Entries[0].Name != "100" {
		t.Errorf("the per-entry diff was lost: %#v", entries[1].Entries)
	}

	limited, err := pg.SyncLog(ctx, 1)
	if err != nil {
		t.Fatalf("SyncLog(1): %v", err)
	}
	if len(limited) != 1 {
		t.Errorf("limit 1 returned %d entries", len(limited))
	}
}

func TestPlanningConditions(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()
	seedSemester(t, pg)

	if err := pg.SetPlanningCondition(ctx, "exams-imported", true); err != nil {
		t.Fatalf("SetPlanningCondition: %v", err)
	}
	// Setting it twice is not an error and does not double it -- presence is the
	// whole state.
	if err := pg.SetPlanningCondition(ctx, "exams-imported", true); err != nil {
		t.Fatalf("SetPlanningCondition again: %v", err)
	}
	if err := pg.SetPlanningCondition(ctx, "rooms-planned", true); err != nil {
		t.Fatalf("SetPlanningCondition: %v", err)
	}

	keys, err := pg.PlanningConditionsSet(ctx)
	if err != nil {
		t.Fatalf("PlanningConditionsSet: %v", err)
	}
	if len(keys) != 2 || keys[0] != "exams-imported" {
		t.Errorf("conditions = %v, want both, sorted", keys)
	}

	if err := pg.SetPlanningCondition(ctx, "rooms-planned", false); err != nil {
		t.Fatalf("SetPlanningCondition(false): %v", err)
	}
	// Unsetting something that is not set is not an error either.
	if err := pg.SetPlanningCondition(ctx, "never-set", false); err != nil {
		t.Fatalf("SetPlanningCondition(false) on nothing: %v", err)
	}

	keys, err = pg.PlanningConditionsSet(ctx)
	if err != nil {
		t.Fatalf("PlanningConditionsSet: %v", err)
	}
	if len(keys) != 1 || keys[0] != "exams-imported" {
		t.Errorf("conditions = %v, want only the remaining one", keys)
	}
}

// TestAssembledExamsStateKeepsReasonAndTime is the schema fix: the table stored
// only `dirty`, but plexams.gui renders the reason and the timestamp in the
// stale banner (Nav.svelte:1024,1027). Same omission as student_regs_state.
func TestAssembledExamsStateKeepsReasonAndTime(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()
	seedSemester(t, pg)

	// No row yet: not dirty, which is what a missing document meant.
	state, err := pg.GetAssembledExamsState(ctx)
	if err != nil {
		t.Fatalf("GetAssembledExamsState: %v", err)
	}
	if state == nil || state.Dirty {
		t.Fatalf("state = %#v, want a non-dirty state", state)
	}

	at := berlin(t, "2027-01-20 08:30")
	if err := pg.SetAssembledExamsDirty(ctx, true, "addPreplanExam", at); err != nil {
		t.Fatalf("SetAssembledExamsDirty: %v", err)
	}

	state, err = pg.GetAssembledExamsState(ctx)
	if err != nil {
		t.Fatalf("GetAssembledExamsState: %v", err)
	}
	if !state.Dirty {
		t.Error("Dirty was not stored")
	}
	if state.Reason == nil || *state.Reason != "addPreplanExam" {
		t.Errorf("Reason = %v, want the operation that marked it stale -- the GUI shows it",
			state.Reason)
	}
	if state.ChangedAt == nil || !state.ChangedAt.Equal(at) {
		t.Errorf("ChangedAt = %v, want %v -- the GUI shows it", state.ChangedAt, at)
	}

	// A regeneration clears the reason with the flag.
	if err := pg.SetAssembledExamsDirty(ctx, false, "", at); err != nil {
		t.Fatalf("SetAssembledExamsDirty(false): %v", err)
	}
	state, err = pg.GetAssembledExamsState(ctx)
	if err != nil {
		t.Fatalf("GetAssembledExamsState: %v", err)
	}
	if state.Dirty || state.Reason != nil {
		t.Errorf("state after regeneration = %#v, want clean and without a reason", state)
	}
	if n := count(t, pg, `select count(*) from assembled_exams_state where semester_id='2026-WS'`); n != 1 {
		t.Errorf("assembled_exams_state rows = %d, want 1", n)
	}
}

func TestEmailAttachments(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()
	seedSemester(t, pg)

	at := berlin(t, "2027-01-20 08:30")
	data := []byte("%PDF-1.4 not really")
	att := &db.EmailAttachment{
		Kind: "cover-page", Key: "180", Filename: "deckblatt.pdf",
		ContentType: "application/pdf", Size: len(data), Data: data, UploadedAt: at,
	}
	if err := pg.SaveEmailAttachment(ctx, att); err != nil {
		t.Fatalf("SaveEmailAttachment: %v", err)
	}
	// Re-uploading replaces it.
	att.Filename = "deckblatt-v2.pdf"
	if err := pg.SaveEmailAttachment(ctx, att); err != nil {
		t.Fatalf("SaveEmailAttachment again: %v", err)
	}

	got, err := pg.GetEmailAttachment(ctx, "cover-page", "180")
	if err != nil {
		t.Fatalf("GetEmailAttachment: %v", err)
	}
	if got == nil || got.Filename != "deckblatt-v2.pdf" {
		t.Fatalf("attachment = %#v, want the replacement", got)
	}
	if string(got.Data) != string(data) {
		t.Errorf("the bytes did not survive the round trip")
	}

	// The listing must not carry the bytes -- it drives a GUI table.
	infos, err := pg.EmailAttachmentInfos(ctx, "cover-page")
	if err != nil {
		t.Fatalf("EmailAttachmentInfos: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("got %d infos, want 1", len(infos))
	}
	if infos[0].Data != nil {
		t.Error("EmailAttachmentInfos returned the binary data")
	}
	if infos[0].Size != len(data) {
		t.Errorf("Size = %d, want %d", infos[0].Size, len(data))
	}

	// A size that does not match the bytes is refused: the document could not say
	// so, and a truncated upload only failed later, when the mail was assembled.
	if err := pg.SaveEmailAttachment(ctx, &db.EmailAttachment{
		Kind: "cover-page", Key: "181", Filename: "x.pdf", Size: 999,
		Data: data, UploadedAt: at,
	}); err == nil {
		t.Error("an attachment whose size disagrees with its bytes was accepted")
	}

	if missing, err := pg.GetEmailAttachment(ctx, "cover-page", "999"); err != nil {
		t.Fatalf("GetEmailAttachment: %v", err)
	} else if missing != nil {
		t.Errorf("GetEmailAttachment = %#v, want nil", missing)
	}

	n, err := pg.ClearEmailAttachments(ctx, "cover-page")
	if err != nil {
		t.Fatalf("ClearEmailAttachments: %v", err)
	}
	if n != 1 {
		t.Errorf("ClearEmailAttachments removed %d, want 1", n)
	}
}
