package graph

import (
	"strings"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/ast"
)

// readOnlyExemptMutations are mutations allowed even on a read-only semester: they
// don't change the active semester's data — only which semester is active, its
// protection, or they create a separate new one.
var readOnlyExemptMutations = map[string]bool{
	"setSemester":         true,
	"setSemesterReadOnly": true,
	"createSemester":      true, // writes into a new, separate semester
}

// todoMutations change only the todo list, never planning data. They stay allowed
// on a read-only semester (so an old semester's todos can still be worked off) and
// while a validation is running. Unlike readOnlyExemptMutations they DO change
// data, so they are not exempt from the VIEWER check. TestTodoMutationsAreListed
// keeps this list in sync with the schema.
var todoMutations = map[string]bool{
	"createTodo":        true,
	"updateTodo":        true,
	"setTodoDone":       true,
	"deleteTodo":        true,
	"addTodoComment":    true,
	"updateTodoComment": true,
	"addTodoLink":       true,
	"removeTodoLink":    true,
	"carryOverTodos":    true,
}

// isTodoOnlyOperation reports whether the operation is a mutation touching
// nothing but todos.
func isTodoOnlyOperation(oc *graphql.OperationContext) bool {
	if oc.Operation == nil || oc.Operation.Operation != ast.Mutation {
		return false
	}
	names := rootFieldNames(oc)
	if len(names) == 0 {
		return false
	}
	for _, name := range names {
		if !todoMutations[name] {
			return false
		}
	}
	return true
}

// isDataChangingOperation reports whether the operation would change the semester's
// data: any mutation (except the read-only-exempt ones) and any subscription that
// is not a read-only validation (validate*). Queries never change data.
//
// A mutation or subscription whose root fields cannot be determined counts as
// data-changing: the gate must fail closed.
func isDataChangingOperation(oc *graphql.OperationContext) bool {
	if oc.Operation == nil {
		return false
	}
	switch oc.Operation.Operation {
	case ast.Mutation:
		names := rootFieldNames(oc)
		if len(names) == 0 {
			return true
		}
		for _, name := range names {
			if !readOnlyExemptMutations[name] {
				return true
			}
		}
		return false
	case ast.Subscription:
		names := rootFieldNames(oc)
		if len(names) == 0 {
			return true
		}
		for _, name := range names {
			if !strings.HasPrefix(name, "validate") {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// rootFieldNames returns the names of the root fields the executor will resolve.
//
// It must collect them exactly as the executor does: a field selected through an
// inline fragment or a fragment spread (`mutation { ... on Mutation { addNTA } }`)
// is executed all the same, so looking only at the direct *ast.Field selections
// let such an operation slip past every check built on these names.
// graphql.CollectFields is the function the generated executor itself uses.
func rootFieldNames(oc *graphql.OperationContext) []string {
	if oc == nil || oc.Operation == nil {
		return nil
	}
	var typeName string
	switch oc.Operation.Operation {
	case ast.Query:
		typeName = "Query"
	case ast.Mutation:
		typeName = "Mutation"
	case ast.Subscription:
		typeName = "Subscription"
	default:
		return nil
	}
	fields := graphql.CollectFields(oc, oc.Operation.SelectionSet, []string{typeName})
	names := make([]string, 0, len(fields))
	for _, f := range fields {
		names = append(names, f.Name)
	}
	return names
}
