package graph

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/obcode/plexams.go/graph/generated"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/apierr"
	"github.com/obcode/plexams.go/principal"
	"github.com/vektah/gqlparser/v2"
)

// parsedOpCtx parses and validates a real document against the schema, so the
// selection set has the shape the executor sees -- including fragments.
func parsedOpCtx(t *testing.T, doc string, vars map[string]any) *graphql.OperationContext {
	t.Helper()
	schema := generated.NewExecutableSchema(generated.Config{}).Schema()
	query, errs := gqlparser.LoadQueryWithRules(schema, doc, nil)
	if errs != nil {
		t.Fatalf("document does not validate: %v", errs)
	}
	if len(query.Operations) != 1 {
		t.Fatalf("want exactly one operation, got %d", len(query.Operations))
	}
	return &graphql.OperationContext{
		RawQuery:  doc,
		Doc:       query,
		Operation: query.Operations[0],
		Variables: vars,
	}
}

func TestRootFieldNamesSeeThroughFragments(t *testing.T) {
	cases := []struct {
		name string
		doc  string
		vars map[string]any
		want []string
	}{
		{"plain", `mutation { setTodoDone(id: 1, done: true) { id } }`, nil, []string{"setTodoDone"}},
		{"alias", `mutation { x: setTodoDone(id: 1, done: true) { id } }`, nil, []string{"setTodoDone"}},
		{"typed inline fragment", `mutation { ... on Mutation { removeUser(email: "a") } }`, nil, []string{"removeUser"}},
		{"untyped inline fragment", `mutation { ... { removeUser(email: "a") } }`, nil, []string{"removeUser"}},
		{"named fragment", `mutation { ...F } fragment F on Mutation { removeUser(email: "a") }`, nil, []string{"removeUser"}},
		{"nested fragments", `mutation { ... on Mutation { ... { removeUser(email: "a") } } }`, nil, []string{"removeUser"}},
		{"todo mixed with planning in a fragment",
			`mutation { setTodoDone(id: 1, done: true) { id } ... on Mutation { removeUser(email: "a") } }`, nil,
			[]string{"setTodoDone", "removeUser"}},
		{"skip excludes", `mutation($s: Boolean!) { removeUser(email: "a") @skip(if: $s) setTodoDone(id: 1, done: true) { id } }`,
			map[string]any{"s": true}, []string{"setTodoDone"}},
		{"subscription in a fragment", `subscription { ... on Subscription { sendEmailExaHM(run: false) { text } } }`, nil,
			[]string{"sendEmailExaHM"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := rootFieldNames(parsedOpCtx(t, c.doc, c.vars))
			if !slices.Equal(got, c.want) {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestFragmentWrappedOperationsAreDataChanging(t *testing.T) {
	for _, doc := range []string{
		`mutation { ... on Mutation { removeUser(email: "a") } }`,
		`mutation { ...F } fragment F on Mutation { removeUser(email: "a") }`,
		`subscription { ... on Subscription { sendEmailExaHM(run: false) { text } } }`,
	} {
		if !isWriteOperation(parsedOpCtx(t, doc, nil)) {
			t.Errorf("%s must count as a write", doc)
		}
	}
	// A todo mutation hidden next to a planning one must not pass as todo-only.
	oc := parsedOpCtx(t, `mutation { setTodoDone(id: 1, done: true) { id } ... on Mutation { removeUser(email: "a") } }`, nil)
	if isTodoOnlyOperation(oc) {
		t.Error("a planning mutation inside a fragment must not count as todo-only")
	}
}

type fakeGateState struct{ writes, readOnly bool }

func (f fakeGateState) WritesAllowed() bool { return f.writes }
func (f fakeGateState) IsReadOnly() bool    { return f.readOnly }

// TestViewerCannotRunAFragmentWrappedMutation drives the real executor: a VIEWER
// sends a mutation wrapped in a fragment. The gate must reject it before any field
// runs -- the tripwire field middleware stands in for the resolver and must never
// be reached.
func TestViewerCannotRunAFragmentWrappedMutation(t *testing.T) {
	var reached atomic.Int32
	srv := newGraphQLServer(generated.NewExecutableSchema(generated.Config{}),
		fakeGateState{writes: true}, nil, true)
	srv.AroundFields(func(ctx context.Context, next graphql.Resolver) (any, error) {
		reached.Add(1)
		return nil, nil
	})

	viewer := &model.User{Email: "viewer@example.test", Name: "V", Role: model.RoleViewer}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.ServeHTTP(w, r.WithContext(principal.WithUser(r.Context(), viewer)))
	})

	for _, doc := range []string{
		`mutation { ... on Mutation { removeUser(email: \"a\") } }`,
		`mutation { ...F } fragment F on Mutation { removeUser(email: \"a\") }`,
	} {
		body := strings.NewReader(`{"query":"` + doc + `"}`)
		req := httptest.NewRequest(http.MethodPost, "/query", body)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if !strings.Contains(rec.Body.String(), `"code":"READ_ONLY_ROLE"`) {
			t.Errorf("%s: want forbidden, got %s", doc, rec.Body.String())
		}
	}
	if n := reached.Load(); n != 0 {
		t.Errorf("a root field was reached %d times", n)
	}
}

func TestWriteOperationKnowsNoExemptions(t *testing.T) {
	cases := []struct {
		doc               string
		write, semesterRW bool
	}{
		{`query { me { email } }`, false, false},
		{`mutation { setSemester(name: "2026-WS") { id } }`, true, false},
		{`mutation { setSemesterReadOnly(readOnly: true) { id } }`, true, false},
		{`mutation { removeUser(email: "a") }`, true, false},
		{`mutation($i: SemesterConfigInputData!) { createSemester(semester: "2027-SS", input: $i) { ok } }`, true, false},
		{`mutation { setMyShortname(shortname: "x") { email } }`, true, false},
		{`mutation { deleteTodo(id: 1) }`, true, true},
		{`subscription { validateConflicts(onlyPlannedByMe: true, ancode: 0) { text } }`, false, false},
		{`subscription { sendEmailExaHM(run: false) { text } }`, true, true},
	}
	for _, c := range cases {
		oc := parsedOpCtx(t, c.doc, nil)
		if got := isWriteOperation(oc); got != c.write {
			t.Errorf("isWriteOperation(%s) = %v, want %v", c.doc, got, c.write)
		}
		if got := isDataChangingOperation(oc); got != c.semesterRW {
			t.Errorf("isDataChangingOperation(%s) = %v, want %v", c.doc, got, c.semesterRW)
		}
	}
}

// postQuery sends one GraphQL document through the gate with the given principal
// (nil = none) and returns the response body. The tripwire counts fields reached.
func postQuery(t *testing.T, state gateState, user *model.User, doc string) (string, int32) {
	t.Helper()
	var reached atomic.Int32
	srv := newGraphQLServer(generated.NewExecutableSchema(generated.Config{}), state, nil, true)
	srv.AroundFields(func(ctx context.Context, next graphql.Resolver) (any, error) {
		reached.Add(1)
		return nil, nil
	})
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user != nil {
			r = r.WithContext(principal.WithUser(r.Context(), user))
		}
		srv.ServeHTTP(w, r)
	})
	payload, _ := json.Marshal(map[string]string{"query": doc})
	req := httptest.NewRequest(http.MethodPost, "/query", strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Body.String(), reached.Load()
}

func TestGate(t *testing.T) {
	viewer := &model.User{Email: "v@example.test", Role: model.RoleViewer}
	planer := &model.User{Email: "p@example.test", Role: model.RolePlaner}
	admin := &model.User{Email: "a@example.test", Role: model.RoleAdmin}
	open := fakeGateState{writes: true}

	cases := []struct {
		name  string
		state gateState
		user  *model.User
		doc   string
		want  string // error code; "" = must reach the field
	}{
		{"no principal, query", open, nil, `query { me { email } }`, codeOf(apierr.CodeForbidden)},
		{"viewer may query", open, viewer, `query { me { email } }`, ""},
		{"viewer may validate", open, viewer, `subscription { validateConflicts(onlyPlannedByMe: true, ancode: 0) { text } }`, ""},
		{"viewer may not switch semester", open, viewer, `mutation { setSemester(name: "2026-WS") { id } }`, codeOf(apierr.CodeReadOnlyRole)},
		{"viewer may not set own shortname", open, viewer, `mutation { setMyShortname(shortname: "x") { email } }`, codeOf(apierr.CodeReadOnlyRole)},
		{"planer may switch semester", open, planer, `mutation { setSemester(name: "2026-WS") { id } }`, ""},
		{"protected semester blocks planning", fakeGateState{writes: true, readOnly: true}, planer,
			`mutation { removeUser(email: "a") ... on Mutation { deleteTodo(id: 1) } }`, codeOf(apierr.CodeSemesterReadOnly)},
		{"protected semester allows user admin", fakeGateState{writes: true, readOnly: true}, admin,
			`mutation { removeUser(email: "a") }`, ""},
		{"viewer may not read the user list", open, viewer, `query { users { email } }`, codeOf(apierr.CodeForbidden)},
		{"planer may not read the user list", open, planer, `query { users { email } }`, codeOf(apierr.CodeForbidden)},
		{"viewer may not reach an admin query through a fragment", open, viewer,
			`query { ... on Query { adminOverview { users { email } } } }`, codeOf(apierr.CodeForbidden)},
		{"protected semester blocks a fragment-wrapped write", fakeGateState{writes: true, readOnly: true}, planer,
			`subscription { ... on Subscription { sendEmailExaHM(run: false) { text } } }`, codeOf(apierr.CodeSemesterReadOnly)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body, reached := postQuery(t, c.state, c.user, c.doc)
			if c.want == "" {
				if reached == 0 {
					t.Errorf("want the field to run, got %s", body)
				}
				return
			}
			if !strings.Contains(body, c.want) || reached != 0 {
				t.Errorf("want %q and no field reached, got %s (reached %d)", c.want, body, reached)
			}
		})
	}
}

// codeOf is how a code appears in a response body. The tests look for the code, not
// the wording, so messages can be reworded without touching them.
func codeOf(code string) string { return `"code":"` + code + `"` }
