package graph

import (
	"testing"

	"github.com/99designs/gqlgen/graphql"
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
		{"createWorkspace exempt", opCtx(ast.Mutation, "createWorkspace"), false},
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
