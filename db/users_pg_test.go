package db_test

import (
	"testing"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
)

func testUser(email string, role model.Role) *model.User {
	return &model.User{
		Email:     email,
		Name:      "Oliver Braun",
		Role:      role,
		Shortname: "obraun",
	}
}

func TestUserRoundTrip(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	want := testUser("oliver.braun@hm.edu", model.RoleAdmin)
	if err := pg.SaveUser(ctx, want); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}

	got, err := pg.GetUserByEmail(ctx, want.Email)
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if got == nil {
		t.Fatal("user is nil")
	}
	if got.Email != want.Email {
		t.Errorf("Email = %q, want %q", got.Email, want.Email)
	}
	if got.Name != want.Name {
		t.Errorf("Name = %q, want %q", got.Name, want.Name)
	}
	if got.Role != want.Role {
		t.Errorf("Role = %q, want %q", got.Role, want.Role)
	}
	if got.Shortname != want.Shortname {
		t.Errorf("Shortname = %q, want %q", got.Shortname, want.Shortname)
	}
}

// TestUserAllRolesRoundTrip walks every value of the Role enum through the check
// constraint, so adding a role to the GraphQL schema without adding it to the
// column fails here rather than at the first login.
func TestUserAllRolesRoundTrip(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	for _, role := range model.AllRole {
		email := string(role) + "@hm.edu"
		if err := pg.SaveUser(ctx, testUser(email, role)); err != nil {
			t.Fatalf("SaveUser(%s): %v", role, err)
		}
		got, err := pg.GetUserByEmail(ctx, email)
		if err != nil {
			t.Fatalf("GetUserByEmail(%s): %v", email, err)
		}
		if got == nil || got.Role != role {
			t.Errorf("Role = %v, want %q", got, role)
		}
	}
}

// The allow-list is fail-closed, so an unknown email must read as "no user" and
// never as an error the middleware might treat as a transient failure.
func TestUserMissingReturnsNilNil(t *testing.T) {
	pg := pgtest.NewDB(t)

	got, err := pg.GetUserByEmail(t.Context(), "nobody@hm.edu")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if got != nil {
		t.Errorf("GetUserByEmail = %v, want nil", got)
	}
}

func TestUserSaveReplaces(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	if err := pg.SaveUser(ctx, testUser("oliver.braun@hm.edu", model.RoleViewer)); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}

	promoted := testUser("oliver.braun@hm.edu", model.RolePlaner)
	promoted.Shortname = ""
	if err := pg.SaveUser(ctx, promoted); err != nil {
		t.Fatalf("SaveUser (second): %v", err)
	}

	users, err := pg.GetUsers(ctx)
	if err != nil {
		t.Fatalf("GetUsers: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("len(GetUsers) = %d, want 1 -- the upsert inserted instead of replacing", len(users))
	}
	if users[0].Role != model.RolePlaner {
		t.Errorf("Role = %q, want %q", users[0].Role, model.RolePlaner)
	}
	if users[0].Shortname != "" {
		t.Errorf("Shortname = %q, want the empty string", users[0].Shortname)
	}
}

func TestUserDelete(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	// Removing a user who was never there is not an error -- the Mongo version
	// ignored the deleted count as well.
	if err := pg.DeleteUser(ctx, "nobody@hm.edu"); err != nil {
		t.Fatalf("DeleteUser (absent): %v", err)
	}

	if err := pg.SaveUser(ctx, testUser("oliver.braun@hm.edu", model.RoleAdmin)); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}
	if err := pg.DeleteUser(ctx, "oliver.braun@hm.edu"); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}

	got, err := pg.GetUserByEmail(ctx, "oliver.braun@hm.edu")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if got != nil {
		t.Error("the user survived the delete")
	}
}

func TestUsersEmptyIsNotNilAndSorted(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	users, err := pg.GetUsers(ctx)
	if err != nil {
		t.Fatalf("GetUsers: %v", err)
	}
	if users == nil {
		t.Fatal("GetUsers returned nil, want an empty slice")
	}

	for _, email := range []string{"zeta@hm.edu", "alpha@hm.edu", "mitte@hm.edu"} {
		if err := pg.SaveUser(ctx, testUser(email, model.RoleViewer)); err != nil {
			t.Fatalf("SaveUser(%s): %v", email, err)
		}
	}

	users, err = pg.GetUsers(ctx)
	if err != nil {
		t.Fatalf("GetUsers: %v", err)
	}
	want := []string{"alpha@hm.edu", "mitte@hm.edu", "zeta@hm.edu"}
	if len(users) != len(want) {
		t.Fatalf("len(GetUsers) = %d, want %d", len(users), len(want))
	}
	for i, w := range want {
		if users[i].Email != w {
			t.Errorf("GetUsers[%d].Email = %q, want %q", i, users[i].Email, w)
		}
	}
}

// An unknown role must not reach the table: the middleware compares against the
// hierarchy by value, and a role it does not know would silently grant nothing --
// or, worse, be treated as a new one later.
func TestUserUnknownRoleIsRejected(t *testing.T) {
	pg := pgtest.NewDB(t)

	err := pg.SaveUser(t.Context(), testUser("oliver.braun@hm.edu", model.Role("SUPERADMIN")))
	if err == nil {
		t.Error("SaveUser accepted a role outside the enum")
	}
}
