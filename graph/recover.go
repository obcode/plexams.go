package graph

import (
	"context"
	"runtime/debug"

	"github.com/99designs/gqlgen/graphql"
	"github.com/rs/zerolog/log"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// recoverFunc replaces gqlgen's graphql.DefaultRecover, which writes the panic
// value and a stack trace straight to os.Stderr with fmt.Fprintln and
// debug.PrintStack (graphql/recovery.go). That bypasses zerolog completely: a
// resolver panic -- the most serious thing that can happen to a request -- ends
// up as unstructured, unlevelled, uncorrelated text that no log query finds and
// no log shipper can parse, while every mundane handled error is properly
// structured. It is also multi-line, so a line-oriented collector splits one
// panic across dozens of records.
//
// Everything here goes through the same logger as the rest of the server. The
// user-facing message is deliberately unchanged from DefaultRecover's: the
// client learns nothing new, only the operator does.
func recoverFunc(ctx context.Context, err any) error {
	event := log.Error().
		Interface("panic", err).
		Str("stack", string(debug.Stack()))

	// HasOperationContext, not GetOperationContext: the latter panics when the
	// context has none, and panicking inside the panic handler would replace a
	// reported bug with an unreported one. A panic during parsing or validation
	// happens before the operation context exists.
	if graphql.HasOperationContext(ctx) {
		// Operation name and field, deliberately not oc.RawQuery or oc.Variables:
		// a query document may carry inline literals (an ancode, a mtknr) and the
		// variables certainly do. The name plus the field plus the stack already
		// pin the bug down, and these logs are meant to be shippable.
		event = event.Str("operation", graphql.GetOperationContext(ctx).OperationName)
	}
	if fc := graphql.GetFieldContext(ctx); fc != nil {
		event = event.Str("field", fc.Field.Name)
	}
	if user := UserFromContext(ctx); user != nil {
		event = event.Str("user", user.Email)
	}

	event.Msg("panic in GraphQL resolver")

	return gqlerror.Errorf("internal system error")
}
