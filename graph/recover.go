package graph

import (
	"context"
	"net/http"
	"runtime/debug"

	"github.com/99designs/gqlgen/graphql"
	"github.com/obcode/plexams.go/obs"
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
	// Report before logging, and from here rather than from anywhere further
	// along: gqlgen calls this from the deferred function that recovered, so
	// runtime.gopanic is still on the stack and the reported trace reaches the
	// resolver that broke. See obs.CapturePanic.
	obs.CapturePanic(ctx, err)

	event := log.Error().
		// The line below stays in the local log but is kept OUT of the error
		// report: obs.CapturePanic just sent this same panic with a usable
		// stack, and the log path would send a second copy whose stack is
		// nothing but zerolog internals.
		Bool(obs.SkipField, true).
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

// recoverMiddleware does for the REST routes what recoverFunc does for the
// resolvers. The ten /upload and /download handlers have no protection of their
// own at all: a panic there unwinds into net/http, which drops the connection
// and writes an unstructured line to raw stderr -- the client sees a reset with
// no status code and the operator sees nothing a log query can find.
//
// It sits outside the CORS and auth middleware, so it also covers those.
func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			// The documented way for a handler to abandon a response without
			// it being a bug. net/http silences it; so do we.
			if rec == http.ErrAbortHandler { //nolint:errorlint // the sentinel is compared by identity, as net/http does
				panic(rec)
			}

			obs.CapturePanic(r.Context(), rec)
			log.Error().
				Bool(obs.SkipField, true). // reported above, with a real stack
				Interface("panic", rec).
				Str("stack", string(debug.Stack())).
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Msg("panic in HTTP handler")

			// Say so, rather than letting the connection die: a 500 is what a
			// client (and the GUI) can act on.
			w.WriteHeader(http.StatusInternalServerError)
		}()

		next.ServeHTTP(w, r)
	})
}
