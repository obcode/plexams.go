package graph

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/principal"
	"github.com/spf13/viper"
)

// fakeAuthProvider implements authProvider without a database.
type fakeAuthProvider struct {
	dev   *model.User
	users map[string]*model.User
}

func (f *fakeAuthProvider) LocalDevUser() *model.User { return f.dev }
func (f *fakeAuthProvider) GetUserByEmail(_ context.Context, email string) (*model.User, error) {
	return f.users[email], nil
}

func TestRoleHierarchy(t *testing.T) {
	// ADMIN ⊇ PLANER ⊇ VIEWER
	if !roleCanWrite(model.RolePlaner) || !roleCanWrite(model.RoleAdmin) {
		t.Error("PLANER and ADMIN must be allowed to write")
	}
	if roleCanWrite(model.RoleViewer) {
		t.Error("VIEWER must be read-only")
	}
	if !roleCanAdmin(model.RoleAdmin) {
		t.Error("ADMIN must be allowed to administer users")
	}
	if roleCanAdmin(model.RolePlaner) || roleCanAdmin(model.RoleViewer) {
		t.Error("only ADMIN may administer users")
	}
}

func TestRequireAdmin(t *testing.T) {
	admin := principal.WithUser(context.Background(), &model.User{Email: "a@hm.edu", Role: model.RoleAdmin})
	if err := requireAdmin(admin); err != nil {
		t.Errorf("admin must pass: %v", err)
	}
	planer := principal.WithUser(context.Background(), &model.User{Email: "p@hm.edu", Role: model.RolePlaner})
	if err := requireAdmin(planer); err == nil {
		t.Error("planer must be rejected by requireAdmin")
	}
	if err := requireAdmin(context.Background()); err == nil {
		t.Error("no principal must be rejected by requireAdmin")
	}
}

func TestUserFromContextAndAuditUser(t *testing.T) {
	ctx := principal.WithUser(context.Background(), &model.User{Email: "a@hm.edu", Role: model.RolePlaner})
	if u := UserFromContext(ctx); u == nil || u.Email != "a@hm.edu" {
		t.Fatalf("UserFromContext = %+v", u)
	}
	// with a principal, auditUser returns its email and never touches p (nil is safe here)
	if got := auditUser(ctx, nil); got == nil || *got != "a@hm.edu" {
		t.Fatalf("auditUser = %v", got)
	}
	if UserFromContext(context.Background()) != nil {
		t.Error("UserFromContext without value must be nil")
	}
}

// capture records the principal the middleware injected.
func capture(seen **model.User) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*seen = UserFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
}

func TestAuthDisabled_InjectsDevUser(t *testing.T) {
	viper.Reset()
	viper.Set("auth.enabled", false)
	p := &fakeAuthProvider{dev: &model.User{Email: "dev@hm.edu", Name: "Dev", Role: model.RolePlaner}}

	var seen *model.User
	h := authMiddleware(p)(capture(&seen))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/query", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if seen == nil || seen.Email != "dev@hm.edu" || seen.Role != model.RolePlaner {
		t.Fatalf("dev user = %+v", seen)
	}
}

func TestAuthEnabled_NoHeader_401(t *testing.T) {
	viper.Reset()
	viper.Set("auth.enabled", true)
	p := &fakeAuthProvider{}

	var seen *model.User
	h := authMiddleware(p)(capture(&seen))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/query", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAuthEnabled_UnknownUser_403(t *testing.T) {
	viper.Reset()
	viper.Set("auth.enabled", true)
	p := &fakeAuthProvider{users: map[string]*model.User{}} // known set is empty → unknown

	var seen *model.User
	h := authMiddleware(p)(capture(&seen))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/query", nil)
	req.Header.Set("X-Remote-User", "stranger@hm.edu")
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestAuthEnabled_KnownUser_Injected(t *testing.T) {
	viper.Reset()
	viper.Set("auth.enabled", true)
	viewer := &model.User{Email: "viewer@hm.edu", Name: "V", Role: model.RoleViewer}
	p := &fakeAuthProvider{users: map[string]*model.User{"viewer@hm.edu": viewer}}

	var seen *model.User
	h := authMiddleware(p)(capture(&seen))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/query", nil)
	req.Header.Set("X-Remote-User", "Viewer@HM.edu") // case-insensitive match
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if seen == nil || seen.Email != "viewer@hm.edu" || seen.Role != model.RoleViewer {
		t.Fatalf("injected user = %+v", seen)
	}
}
