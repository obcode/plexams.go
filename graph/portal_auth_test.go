package graph

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/principal"
	"github.com/spf13/viper"
)

func portalTestAuth() *fakeAuthProvider {
	return &fakeAuthProvider{
		users: map[string]*model.User{
			"planer@example.test": {Email: "planer@example.test", Name: "P", Role: model.RolePlaner},
		},
		portal: map[string]*db.PortalLogin{
			"teacher@example.test": {TeacherIDs: []int{10}, TeacherNames: map[int]string{10: "Person T"}},
			"planer@example.test":  {TeacherIDs: []int{20}},
			"blocked@example.test": {TeacherIDs: []int{30}, Blocked: true},
			"student@example.test": {TeacherIDs: []int{40}}, // an address that matches anyway
		},
	}
}

func withConfig(t *testing.T, kv map[string]any) {
	t.Helper()
	for k, v := range kv {
		viper.Set(k, v)
	}
	t.Cleanup(func() {
		for k := range kv {
			viper.Set(k, nil)
		}
	})
}

// captured runs one request through authMiddleware and returns the status and the
// principal the next handler saw.
func captured(t *testing.T, auth authProvider, headers map[string]string) (int, *principal.Principal) {
	t.Helper()
	var seen *principal.Principal
	h := authMiddleware(auth)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = principal.From(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/query", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, seen
}

func TestPortalOffKeepsTodaysAllowList(t *testing.T) {
	withConfig(t, map[string]any{"auth.enabled": true, "portal.enabled": false})
	code, _ := captured(t, portalTestAuth(), map[string]string{"X-Remote-User": "teacher@example.test"})
	if code != http.StatusForbidden {
		t.Errorf("with the portal off a teacher without app_user row gets 403, got %d", code)
	}
	code, pr := captured(t, portalTestAuth(), map[string]string{"X-Remote-User": "planer@example.test"})
	if code != http.StatusOK || pr.Staff == nil || len(pr.TeacherIDs) != 0 {
		t.Errorf("with the portal off the planner is staff only, got %d %+v", code, pr)
	}
}

func TestPortalOnResolvesTeachers(t *testing.T) {
	withConfig(t, map[string]any{"auth.enabled": true, "portal.enabled": true})

	code, pr := captured(t, portalTestAuth(), map[string]string{"X-Remote-User": " Teacher@Example.test "})
	if code != http.StatusOK || pr.Staff != nil || !slices.Equal(pr.TeacherIDs, []int{10}) || pr.Name != "Person T" {
		t.Errorf("teacher-only login: got %d %+v", code, pr)
	}
	code, pr = captured(t, portalTestAuth(), map[string]string{"X-Remote-User": "planer@example.test"})
	if code != http.StatusOK || pr.Staff == nil || !slices.Equal(pr.TeacherIDs, []int{20}) {
		t.Errorf("planner who is also a teacher: got %d %+v", code, pr)
	}
	code, _ = captured(t, portalTestAuth(), map[string]string{"X-Remote-User": "blocked@example.test"})
	if code != http.StatusForbidden {
		t.Errorf("a blocked login without staff role gets 403, got %d", code)
	}
	code, _ = captured(t, portalTestAuth(), map[string]string{"X-Remote-User": "stranger@example.test"})
	if code != http.StatusForbidden {
		t.Errorf("an unknown login gets 403, got %d", code)
	}
}

func TestStudentsAreTurnedAwayOnlyWhenSwitchedOn(t *testing.T) {
	headers := map[string]string{"X-Remote-User": "student@example.test", "X-Remote-Affiliation": "student@hm.edu"}

	withConfig(t, map[string]any{"auth.enabled": true, "portal.enabled": true, "portal.denystudents": false})
	if code, _ := captured(t, portalTestAuth(), headers); code != http.StatusOK {
		t.Errorf("without portal.denystudents the claim is ignored, got %d", code)
	}
	viper.Set("portal.denystudents", true)
	if code, _ := captured(t, portalTestAuth(), headers); code != http.StatusForbidden {
		t.Errorf("a student-only affiliation is turned away, got %d", code)
	}
	headers["X-Remote-Affiliation"] = "student@hm.edu,employee@hm.edu"
	if code, _ := captured(t, portalTestAuth(), headers); code != http.StatusOK {
		t.Errorf("a student who is also employed is not student-only, got %d", code)
	}
}

func TestStudentOnly(t *testing.T) {
	for value, want := range map[string]bool{
		"":                               false,
		"student@hm.edu":                 true,
		" Student@HM.EDU , student@x":    true,
		"employee@hm.edu":                false,
		"member@hm.edu,student@hm.edu":   false,
		"faculty@hm.edu,employee@hm.edu": false,
	} {
		if got := studentOnly(value); got != want {
			t.Errorf("studentOnly(%q) = %v, want %v", value, got, want)
		}
	}
}

func TestPortalOnlyLoginIsShutOutOfThePlannerAPI(t *testing.T) {
	withConfig(t, map[string]any{"auth.enabled": true, "portal.enabled": true})
	graphQL := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	router := newRouter(portalTestAuth(), fakeGateState{writes: true}, graphQL, stubRoutes(), nil, true)

	for _, c := range []struct{ method, path string }{
		{"POST", "/query"}, {"GET", "/query"}, {"GET", "/download/pdf/draft-fs"},
		{"POST", "/upload/dataset-csv"}, {"POST", "/upload/jira-attachment"},
	} {
		req := httptest.NewRequest(c.method, c.path, nil)
		req.Header.Set("X-Remote-User", "teacher@example.test")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s as portal-only login: got %d, want 403", c.method, c.path, rec.Code)
		}
	}
	req := httptest.NewRequest("POST", "/query", nil)
	req.Header.Set("X-Remote-User", "planer@example.test")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("a planner still reaches /query, got %d", rec.Code)
	}
}

func TestRejectedLoginsAreRemembered(t *testing.T) {
	withConfig(t, map[string]any{"auth.enabled": true, "portal.enabled": true})
	captured(t, portalTestAuth(), map[string]string{"X-Remote-User": "rejected-a@example.test"})
	captured(t, portalTestAuth(), map[string]string{"X-Remote-User": "rejected-b@example.test"})
	captured(t, portalTestAuth(), map[string]string{"X-Remote-User": "rejected-a@example.test"})
	list := rejectedLogins.list()
	if len(list) < 2 || list[0].Email != "rejected-a@example.test" || list[0].Count != 2 {
		t.Errorf("want the latest rejection first with its count, got %+v", list[:min(len(list), 3)])
	}
}

func TestDevPrincipalCanPlayEveryPart(t *testing.T) {
	auth := portalTestAuth()
	auth.dev = &model.User{Email: "dev@example.test", Name: "Dev", Role: model.RoleAdmin}
	withConfig(t, map[string]any{"auth.enabled": false, "portal.enabled": true})

	if _, pr := captured(t, auth, nil); pr.Staff == nil || pr.Staff.Role != model.RoleAdmin {
		t.Errorf("default dev principal is ADMIN, got %+v", pr)
	}
	viper.Set("auth.devrole", "NONE")
	viper.Set("auth.devteacherids", []int{10})
	t.Cleanup(func() { viper.Set("auth.devrole", nil); viper.Set("auth.devteacherids", nil) })
	if _, pr := captured(t, auth, nil); pr.Staff != nil || !slices.Equal(pr.TeacherIDs, []int{10}) {
		t.Errorf("devrole NONE with a dev teacher id: got %+v", pr)
	}
	if auth.dev.Role != model.RoleAdmin {
		t.Error("changing the dev role must not modify the shared dev user")
	}
}
