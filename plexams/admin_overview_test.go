package plexams

import (
	"testing"
	"time"

	"github.com/obcode/plexams.go/graph/model"
)

// ptrStr is a tiny helper for the optional *string fields on MutationLogEntry.
func ptrStr(s string) *string { return &s }

func TestComputeActivitySummary(t *testing.T) {
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.Local)
	errMsg := "boom"
	entries := []*model.MutationLogEntry{
		{Time: now.Add(-1 * time.Hour), Name: "addPreplanExam", User: ptrStr("a@hm.edu")},
		{Time: now.Add(-2 * time.Hour), Name: "addPreplanExam", User: ptrStr("a@hm.edu"), Error: &errMsg},
		{Time: now.Add(-30 * time.Hour), Name: "setPlanningCondition", User: ptrStr("b@hm.edu")}, // >24h, <7d
		{Time: now.Add(-30 * time.Hour), Name: "addPreplanExam", User: ptrStr("a@hm.edu")},
		{Time: now.Add(-8 * 24 * time.Hour), Name: "old", User: ptrStr("c@hm.edu")}, // outside window: ignored
		nil, // defensively skipped
	}

	got := computeActivitySummary(entries, now)

	if got.Last24h != 2 {
		t.Errorf("Last24h = %d, want 2", got.Last24h)
	}
	if got.Last7d != 4 {
		t.Errorf("Last7d = %d, want 4", got.Last7d)
	}
	if got.Errors7d != 1 {
		t.Errorf("Errors7d = %d, want 1", got.Errors7d)
	}
	if got.DistinctUsers7d != 2 { // a@ and b@ (c@ is outside the window)
		t.Errorf("DistinctUsers7d = %d, want 2", got.DistinctUsers7d)
	}
	if len(got.TopOperations) != 2 {
		t.Fatalf("TopOperations len = %d, want 2", len(got.TopOperations))
	}
	if got.TopOperations[0].Name != "addPreplanExam" || got.TopOperations[0].Count != 3 {
		t.Errorf("top op = %+v, want addPreplanExam x3", got.TopOperations[0])
	}
}

func TestComputeActivitySummaryEmpty(t *testing.T) {
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.Local)
	got := computeActivitySummary(nil, now)
	if got.Last24h != 0 || got.Last7d != 0 || got.Errors7d != 0 || got.DistinctUsers7d != 0 {
		t.Errorf("empty summary not zero: %+v", got)
	}
	if got.TopOperations == nil {
		t.Error("TopOperations should be non-nil (empty slice), got nil")
	}
}

func TestRoleCounts(t *testing.T) {
	users := []*model.User{
		{Email: "admin@hm.edu", Role: model.RoleAdmin},
		{Email: "p1@hm.edu", Role: model.RolePlaner},
		{Email: "p2@hm.edu", Role: model.RolePlaner},
		{Email: "v@hm.edu", Role: model.RoleViewer},
	}
	c := roleCounts(users)
	if c.Total != 4 || c.Admin != 1 || c.Planer != 2 || c.Viewer != 1 {
		t.Errorf("roleCounts = %+v", c)
	}
}

func TestAdminRecipientsFromUsers(t *testing.T) {
	admins := []*model.User{
		{Email: "a@hm.edu", Role: model.RoleAdmin},
		{Email: "b@hm.edu", Role: model.RolePlaner},
		{Email: "c@hm.edu", Role: model.RoleAdmin},
		{Email: "", Role: model.RoleAdmin}, // empty email skipped
	}

	got := adminRecipientsFromUsers(admins, "fallback@hm.edu")
	if len(got) != 2 || got[0] != "a@hm.edu" || got[1] != "c@hm.edu" {
		t.Errorf("admins = %v, want [a@hm.edu c@hm.edu]", got)
	}

	// No admins -> first non-empty fallback.
	noAdmins := []*model.User{{Email: "b@hm.edu", Role: model.RolePlaner}}
	got = adminRecipientsFromUsers(noAdmins, "", "  ", "fb@hm.edu", "other@hm.edu")
	if len(got) != 1 || got[0] != "fb@hm.edu" {
		t.Errorf("fallback = %v, want [fb@hm.edu]", got)
	}

	// No admins, no fallback -> empty (caller skips the send).
	if got := adminRecipientsFromUsers(nil, "", ""); len(got) != 0 {
		t.Errorf("empty = %v, want []", got)
	}
}

func TestFailedEntriesAndFirstN(t *testing.T) {
	e := "err"
	entries := []*model.MutationLogEntry{
		{Name: "ok1"},
		{Name: "bad", Error: &e},
		{Name: "ok2", Error: ptrStr("")}, // empty error string is not a failure
	}
	failed := failedEntries(entries)
	if len(failed) != 1 || failed[0].Name != "bad" {
		t.Errorf("failedEntries = %+v", failed)
	}
	if got := firstN(entries, 2); len(got) != 2 {
		t.Errorf("firstN(2) len = %d, want 2", len(got))
	}
	if got := firstN(entries, 10); len(got) != 3 {
		t.Errorf("firstN(10) len = %d, want 3", len(got))
	}
	if got := firstN(nil, 5); got == nil || len(got) != 0 {
		t.Errorf("firstN(nil) = %v, want []", got)
	}
}
