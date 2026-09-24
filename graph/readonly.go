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
	names := rootFieldNames(oc.Operation.SelectionSet)
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
func isDataChangingOperation(oc *graphql.OperationContext) bool {
	if oc.Operation == nil {
		return false
	}
	switch oc.Operation.Operation {
	case ast.Mutation:
		for _, name := range rootFieldNames(oc.Operation.SelectionSet) {
			if !readOnlyExemptMutations[name] {
				return true
			}
		}
		return false
	case ast.Subscription:
		for _, name := range rootFieldNames(oc.Operation.SelectionSet) {
			if !strings.HasPrefix(name, "validate") {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func rootFieldNames(set ast.SelectionSet) []string {
	names := make([]string, 0, len(set))
	for _, sel := range set {
		if field, ok := sel.(*ast.Field); ok {
			names = append(names, field.Name)
		}
	}
	return names
}
