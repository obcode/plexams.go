package db_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
)

func seedTeachersIn(t *testing.T, pg *db.PG, semester string, teachers ...*model.Teacher) {
	t.Helper()
	exec(t, pg, `insert into semester (id, schema_version) values ('`+semester+`', 2) on conflict do nothing`)
	pg.SwitchTo(t.Context(), semester)
	if err := pg.CacheTeachers(teachers, strings.Replace(semester, "-", " ", 1)); err != nil {
		t.Fatal(err)
	}
}

func TestResolvePortalLogin(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	// The same person in three semesters, with an address that changed; a second person
	// only in an old snapshot; the "nobody" id 0 with a real-looking address.
	seedTeachersIn(t, pg, "2025-SS", &model.Teacher{ID: 20, Fullname: "Old Only", Email: "old@example.test"})
	seedTeachersIn(t, pg, "2025-WS", &model.Teacher{ID: 10, Fullname: "Person A", Email: "a.old@example.test"})
	seedTeachersIn(t, pg, "2026-SS", &model.Teacher{ID: 10, Fullname: "Person A", Email: " A@Example.test "})
	seedTeachersIn(t, pg, "2026-WS",
		&model.Teacher{ID: 10, Fullname: "Person A", Email: "a@example.test"},
		&model.Teacher{ID: 0, Fullname: "N. N.", Email: "nobody@example.test"})

	// The active semester must not matter.
	pg.SwitchTo(ctx, "2025-SS")

	login, err := pg.ResolvePortalLogin(ctx, "a@example.test", 3)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(login.TeacherIDs, []int{10}) || login.TeacherNames[10] != "Person A" {
		t.Errorf("want teacher 10, got %+v", login)
	}

	// Outside the three newest snapshots: not matched.
	if login, _ := pg.ResolvePortalLogin(ctx, "old@example.test", 3); len(login.TeacherIDs) != 0 {
		t.Errorf("an address only in an old snapshot must not match, got %v", login.TeacherIDs)
	}
	// Id 0 is nobody.
	if login, _ := pg.ResolvePortalLogin(ctx, "nobody@example.test", 3); len(login.TeacherIDs) != 0 {
		t.Errorf("id 0 must never be a login's identity, got %v", login.TeacherIDs)
	}

	// Alias, block, delegation (one valid, one expired), grant.
	if err := pg.SetLoginAlias(ctx, "alias@example.test", 10, "SSO differs", "admin@example.test"); err != nil {
		t.Fatal(err)
	}
	if err := pg.SetPortalLoginBlock(ctx, "alias@example.test", "test", "admin@example.test"); err != nil {
		t.Fatal(err)
	}
	exec(t, pg, `insert into portal_delegation (delegate_email, teacher_id, created_by) values ('alias@example.test', 20, 'x')`)
	exec(t, pg, `insert into portal_delegation (delegate_email, teacher_id, valid_until, created_by) values ('alias@example.test', 30, current_date - 1, 'x')`)
	exec(t, pg, `insert into study_program (shortname, name, category, active, joint_faculty) values ('DE', 'Digital Engineering', 'joint', true, 'MUC.DAI')`)
	exec(t, pg, `insert into program_grant (email, program, level, created_by) values ('alias@example.test', 'DE', 'READ', 'x')`)

	login, err = pg.ResolvePortalLogin(ctx, "alias@example.test", 3)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(login.TeacherIDs, []int{10}) || !login.Blocked ||
		!slices.Equal(login.ActsFor, []int{20}) || login.Grants["DE"] != "READ" {
		t.Errorf("unexpected login %+v", login)
	}
}

func TestPortalAccessTablesRejectBadValues(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()
	for _, bad := range []struct {
		email string
		id    int
	}{{"Upper@example.test", 1}, {" x@example.test", 1}, {"x@example.test", 0}} {
		if err := pg.SetLoginAlias(ctx, bad.email, bad.id, "", "a"); err == nil {
			t.Errorf("alias %q -> %d must be rejected", bad.email, bad.id)
		}
	}
}

func TestPortalTokenLifecycle(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()
	tok := &db.PortalToken{
		TokenID: "0123456789ABCDEF", OwnerEmail: "a@example.test", Description: "script",
		Scope: "READ", SecretHash: make([]byte, 32), ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := pg.InsertPortalToken(ctx, tok); err != nil {
		t.Fatal(err)
	}
	got, err := pg.PortalTokenByID(ctx, tok.TokenID)
	if err != nil || got == nil || got.OwnerEmail != "a@example.test" || got.RevokedAt != nil {
		t.Fatalf("got %+v, %v", got, err)
	}
	if missing, err := pg.PortalTokenByID(ctx, "ZZZZZZZZZZZZZZZZ"); missing != nil || err != nil {
		t.Errorf("unknown token: got %+v, %v", missing, err)
	}

	if err := pg.MarkPortalTokenUsed(ctx, tok.TokenID); err != nil {
		t.Fatal(err)
	}
	if got, _ := pg.PortalTokenByID(ctx, tok.TokenID); got.LastUsedAt == nil {
		t.Error("last use not recorded")
	}

	if ok, _ := pg.RevokePortalTokenOf(ctx, tok.TokenID, "b@example.test"); ok {
		t.Error("someone else must not revoke the token")
	}
	if ok, _ := pg.RevokePortalTokenOf(ctx, tok.TokenID, "a@example.test"); !ok {
		t.Error("the owner must be able to revoke")
	}
	if ok, _ := pg.RevokePortalTokenOf(ctx, tok.TokenID, "a@example.test"); ok {
		t.Error("revoking twice reports nothing revoked")
	}

	bad := *tok
	bad.TokenID = "lowercase-not-ok"
	if err := pg.InsertPortalToken(ctx, &bad); err == nil {
		t.Error("a malformed token id must be rejected")
	}
}
