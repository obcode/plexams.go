package graph

import (
	"strings"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/ast"
)

// readOnlyExemptMutations are mutations allowed even on a read-only database: they
// don't change the active database's data — only which database is active, its
// protection, or they create a separate new database.
var readOnlyExemptMutations = map[string]bool{
	"setSemester":         true,
	"setSemesterReadOnly": true,
	"createWorkspace":     true, // writes into a new, separate database
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
