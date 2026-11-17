package graph

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/obcode/plexams.go/graph/generated"
	"github.com/obcode/plexams.go/graph/model"
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
		if !isDataChangingOperation(parsedOpCtx(t, doc, nil)) {
			t.Errorf("%s must count as data-changing", doc)
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

		if !strings.Contains(rec.Body.String(), "forbidden: your role is read-only") {
			t.Errorf("%s: want forbidden, got %s", doc, rec.Body.String())
		}
	}
	if n := reached.Load(); n != 0 {
		t.Errorf("a root field was reached %d times", n)
	}
}
