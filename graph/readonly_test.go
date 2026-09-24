package graph

import (
	"strings"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/obcode/plexams.go/graph/generated"
	"github.com/vektah/gqlparser/v2/ast"
)

func opCtx(op ast.Operation, fields ...string) *graphql.OperationContext {
	sel := ast.SelectionSet{}
	for _, f := range fields {
		sel = append(sel, &ast.Field{Name: f})
	}
	return &graphql.OperationContext{
		Operation: &ast.OperationDefinition{Operation: op, SelectionSet: sel},
	}
}

func TestIsDataChangingOperation(t *testing.T) {
	cases := []struct {
		name string
		oc   *graphql.OperationContext
		want bool
	}{
		{"query never changes", opCtx(ast.Query, "students"), false},
		{"normal mutation changes", opCtx(ast.Mutation, "addNTA"), true},
		{"setSemester exempt", opCtx(ast.Mutation, "setSemester"), false},
		{"setSemesterReadOnly exempt", opCtx(ast.Mutation, "setSemesterReadOnly"), false},
		{"createSemester exempt", opCtx(ast.Mutation, "createSemester"), false},
		{"mixed mutation changes", opCtx(ast.Mutation, "setSemester", "addNTA"), true},
		{"validation subscription ok", opCtx(ast.Subscription, "validateConflicts"), false},
		{"import subscription changes", opCtx(ast.Subscription, "importExamsFromZPA"), true},
	}
	for _, c := range cases {
		if got := isDataChangingOperation(c.oc); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestIsTodoOnlyOperation(t *testing.T) {
	cases := []struct {
		name string
		oc   *graphql.OperationContext
		want bool
	}{
		{"todo mutation", opCtx(ast.Mutation, "createTodo"), true},
		{"several todo mutations", opCtx(ast.Mutation, "setTodoDone", "addTodoComment"), true},
		{"todo mixed with planning", opCtx(ast.Mutation, "createTodo", "addNTA"), false},
		{"planning mutation", opCtx(ast.Mutation, "addNTA"), false},
		{"todo query is no mutation", opCtx(ast.Query, "todos"), false},
		{"empty mutation", opCtx(ast.Mutation), false},
	}
	for _, c := range cases {
		if got := isTodoOnlyOperation(c.oc); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
	// A todo mutation still changes data: the VIEWER check must keep catching it.
	if !isDataChangingOperation(opCtx(ast.Mutation, "createTodo")) {
		t.Error("createTodo must count as data-changing (VIEWERs may not write todos)")
	}
}

// TestTodoMutationsAreListed fails when a todo mutation is added to the schema but
// not to todoMutations -- it would then be blocked on a read-only semester.
func TestTodoMutationsAreListed(t *testing.T) {
	schema := generated.NewExecutableSchema(generated.Config{}).Schema()
	for _, field := range schema.Mutation.Fields {
		if strings.Contains(field.Name, "Todo") && !todoMutations[field.Name] {
			t.Errorf("mutation %s is missing from todoMutations", field.Name)
		}
	}
	for name := range todoMutations {
		if schema.Mutation.Fields.ForName(name) == nil {
			t.Errorf("todoMutations lists %s, which is not in the schema", name)
		}
	}
}
