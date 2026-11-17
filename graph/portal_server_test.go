package graph

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
	"github.com/obcode/plexams.go/plexams/inquiry"
	"github.com/obcode/plexams.go/plexams/portal"
)

type portalHarness struct {
	t      *testing.T
	router http.Handler
	pg     *db.PG
}

func newPortalHarness(t *testing.T, mount bool, tokens bool) *portalHarness {
	t.Helper()
	pg := pgtest.NewDBWithSemester(t)
	if err := pg.CacheTeachers([]*model.Teacher{
		{ID: 10, Fullname: "Person A", Shortname: "A", Email: "teacher@example.test", IsProf: true, FK: "FK07", IsActive: true},
	}, "2026 WS"); err != nil {
		t.Fatal(err)
	}
	withConfig(t, map[string]any{"auth.enabled": true, "portal.enabled": mount})
	service := portal.New(pg)
	t.Cleanup(service.Wait)

	// The real auth provider shape: app_user via the fake, portal tables via the DB.
	auth := &dbPortalAuth{fakeAuthProvider: fakeAuthProvider{users: map[string]*model.User{
		"planer@example.test": {Email: "planer@example.test", Name: "P", Role: model.RolePlaner},
	}}, store: pg}

	var m *portalMount
	if mount {
		m = &portalMount{service: service, inquiries: inquiry.New(inquiry.NewStore(pg)), tokens: tokens}
	}
	planner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	return &portalHarness{t: t, pg: pg, router: newRouter(auth, fakeGateState{writes: true}, planner, stubRoutes(), nil, true, m)}
}

// dbPortalAuth answers ResolvePortalLogin from the real tables.
type dbPortalAuth struct {
	fakeAuthProvider
	store portal.Store
}

func (d *dbPortalAuth) ResolvePortalLogin(ctx context.Context, email string) (*db.PortalLogin, error) {
	return d.store.ResolvePortalLogin(ctx, email, 3)
}

type gqlResult struct {
	status int
	Data   map[string]any `json:"data"`
	Errors []struct {
		Message    string         `json:"message"`
		Extensions map[string]any `json:"extensions"`
	} `json:"errors"`
}

func (r gqlResult) code() string {
	if len(r.Errors) == 0 {
		return ""
	}
	c, _ := r.Errors[0].Extensions["code"].(string)
	return c
}

func (h *portalHarness) post(path string, headers map[string]string, query string, vars map[string]any) gqlResult {
	h.t.Helper()
	body, _ := json.Marshal(map[string]any{"query": query, "variables": vars})
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)
	res := gqlResult{status: rec.Code}
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
			h.t.Fatalf("%s: %v: %s", path, err, rec.Body.String())
		}
	}
	return res
}

const createToken = `mutation($s: TokenScope!) { createAccessToken(description: "test", scope: $s) { secret token { id scope } } }`

func TestPortalRoutesExistOnlyWhenEnabled(t *testing.T) {
	h := newPortalHarness(t, false, false)
	for _, path := range []string{portalPath, tokenPath} {
		if res := h.post(path, map[string]string{"X-Remote-User": "planer@example.test"}, `{ session { email } }`, nil); res.status != http.StatusNotFound {
			t.Errorf("%s with the portal off: got %d, want 404", path, res.status)
		}
	}
}

func TestPortalSessionDoor(t *testing.T) {
	h := newPortalHarness(t, true, true)
	teacher := map[string]string{"X-Remote-User": "teacher@example.test"}

	res := h.post(portalPath, teacher, `{ session { email isStaff teachers { id fullname } } }`, nil)
	if res.status != http.StatusOK || len(res.Errors) != 0 {
		t.Fatalf("session: %+v", res)
	}
	s := res.Data["session"].(map[string]any)
	if s["email"] != "teacher@example.test" || s["isStaff"] != false ||
		s["teachers"].([]any)[0].(map[string]any)["fullname"] != "Person A" {
		t.Errorf("unexpected session %+v", s)
	}

	if res := h.post(portalPath, map[string]string{"X-Remote-User": "stranger@example.test"}, `{ session { email } }`, nil); res.status != http.StatusForbidden {
		t.Errorf("unknown login: got %d, want 403", res.status)
	}
	// The teacher is shut out of the planner API.
	if res := h.post("/query", teacher, `{ me { email } }`, nil); res.status != http.StatusForbidden {
		t.Errorf("teacher on /query: got %d, want 403", res.status)
	}
	// Introspection works for tooling.
	if res := h.post(portalPath, teacher, `{ __schema { queryType { name } } }`, nil); len(res.Errors) != 0 {
		t.Errorf("introspection: %+v", res.Errors)
	}
}

func TestPortalTokenDoor(t *testing.T) {
	h := newPortalHarness(t, true, true)
	teacher := map[string]string{"X-Remote-User": "teacher@example.test"}

	mint := func(scope string) string {
		res := h.post(portalPath, teacher, createToken, map[string]any{"s": scope})
		if len(res.Errors) != 0 {
			t.Fatalf("createAccessToken: %+v", res.Errors)
		}
		return res.Data["createAccessToken"].(map[string]any)["secret"].(string)
	}
	read := mint("READ")
	write := mint("WRITE")
	bearer := func(secret string) map[string]string { return map[string]string{"Authorization": "Bearer " + secret} }

	res := h.post(tokenPath, bearer(read), `{ session { email isStaff token { scope } } }`, nil)
	if len(res.Errors) != 0 || res.Data["session"].(map[string]any)["token"].(map[string]any)["scope"] != "READ" {
		t.Fatalf("session with a token: %+v", res)
	}

	if res := h.post(tokenPath, bearer(read), `mutation { revokeAccessToken(id: "0123456789ABCDEF") }`, nil); res.code() != "READ_ONLY_TOKEN" {
		t.Errorf("READ token running a mutation: want READ_ONLY_TOKEN, got %+v", res)
	}
	if res := h.post(tokenPath, bearer(write), createToken, map[string]any{"s": "WRITE"}); res.code() != "INTERACTIVE_ONLY" {
		t.Errorf("a token minting a token: want INTERACTIVE_ONLY, got %+v", res)
	}
	if res := h.post(tokenPath, bearer(read), `{ accessTokens { id } }`, nil); res.code() != "INTERACTIVE_ONLY" {
		t.Errorf("a token listing tokens: want INTERACTIVE_ONLY, got %+v", res)
	}

	// No fallback to the proxy header on the token door, and no token on the others.
	if res := h.post(tokenPath, teacher, `{ session { email } }`, nil); res.status != http.StatusUnauthorized {
		t.Errorf("X-Remote-User on the token door: got %d, want 401", res.status)
	}
	if res := h.post(tokenPath, bearer("plexams_nonsense"), `{ session { email } }`, nil); res.status != http.StatusUnauthorized {
		t.Errorf("a malformed token: got %d, want 401", res.status)
	}
	for _, path := range []string{"/query", portalPath, "/download/pdf/draft-fs"} {
		if res := h.post(path, bearer(read), `{ session { email } }`, nil); res.status != http.StatusUnauthorized {
			t.Errorf("a token on %s: got %d, want 401", path, res.status)
		}
	}
}

func TestTokenDoorCanBeSwitchedOff(t *testing.T) {
	h := newPortalHarness(t, true, false)
	if res := h.post(tokenPath, map[string]string{"Authorization": "Bearer x"}, `{ session { email } }`, nil); res.status != http.StatusNotFound {
		t.Errorf("token door with portal.tokens off: got %d, want 404", res.status)
	}
	if res := h.post(portalPath, map[string]string{"X-Remote-User": "teacher@example.test"}, `{ session { email } }`, nil); res.status != http.StatusOK {
		t.Errorf("the session door stays: got %d", res.status)
	}
}

func TestRateLimiter(t *testing.T) {
	l := newRateLimiter(1, 3)
	for i := range 3 {
		if !l.allow("k") {
			t.Fatalf("request %d within the burst refused", i)
		}
	}
	if l.allow("k") {
		t.Error("a request beyond the burst must wait")
	}
	if !l.allow("other") {
		t.Error("keys are independent")
	}
}

func TestPortalPreplanThroughBothDoors(t *testing.T) {
	h := newPortalHarness(t, true, true)
	ctx := context.Background()
	if err := h.pg.UpsertStudyProgram(ctx, &model.StudyProgram{Shortname: "IF", Name: "Informatik", Category: "fk07"}); err != nil {
		t.Fatal(err)
	}
	if err := h.pg.EnsureSemester(ctx, "2027-SS", db.CurrentSchemaVersion); err != nil {
		t.Fatal(err)
	}
	svc := inquiry.New(inquiry.NewStore(h.pg))
	if _, err := svc.SetInquiry(ctx, "2027-SS", inquiry.KindPreplan, nil, "Bitte melden"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenInquiry(ctx, "2027-SS", inquiry.KindPreplan, "planer@example.test"); err != nil {
		t.Fatal(err)
	}
	teacher := map[string]string{"X-Remote-User": "teacher@example.test"}

	res := h.post(portalPath, teacher, `{ inquiries { semester kind open affected } studyPrograms { shortname } }`, nil)
	if len(res.Errors) != 0 {
		t.Fatalf("inquiries: %+v", res.Errors)
	}
	inqs := res.Data["inquiries"].([]any)
	if len(inqs) != 1 || inqs[0].(map[string]any)["affected"] != true {
		t.Errorf("unexpected inquiries %+v", inqs)
	}

	const submit = `mutation($in: PreplanInput!) { submitPreplan(semester: "2027-SS", input: $in) { id revision module teacher { id } } }`
	in := map[string]any{"in": map[string]any{"examKind": "SEB", "module": "Web", "programs": []string{"IF"}, "expectedStudents": 30}}
	res = h.post(portalPath, teacher, submit, in)
	if len(res.Errors) != 0 || res.Data["submitPreplan"].(map[string]any)["module"] != "Web" {
		t.Fatalf("submit via session: %+v", res)
	}

	mint := func(scope string) map[string]string {
		res := h.post(portalPath, teacher, createToken, map[string]any{"s": scope})
		return map[string]string{"Authorization": "Bearer " + res.Data["createAccessToken"].(map[string]any)["secret"].(string)}
	}
	if res := h.post(tokenPath, mint("READ"), submit, in); res.code() != "READ_ONLY_TOKEN" {
		t.Errorf("READ token submitting: want READ_ONLY_TOKEN, got %+v", res)
	}
	write := mint("WRITE")
	if res := h.post(tokenPath, write, submit, in); len(res.Errors) != 0 {
		t.Errorf("WRITE token submitting: %+v", res.Errors)
	}
	res = h.post(tokenPath, write, `{ preplanSubmissions(semester: "2027-SS") { id } }`, nil)
	if len(res.Errors) != 0 || len(res.Data["preplanSubmissions"].([]any)) != 2 {
		t.Errorf("listing via token: %+v", res)
	}
	bad := map[string]any{"in": map[string]any{"examKind": "SEB", "module": "Web", "programs": []string{"XX"}, "expectedStudents": 30}}
	if res := h.post(portalPath, teacher, submit, bad); res.code() != "INVALID_INPUT" {
		t.Errorf("unknown program: want INVALID_INPUT, got %+v", res)
	}
}
