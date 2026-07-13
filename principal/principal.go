// Package principal carries the authenticated user through the request context in a
// place both the graph (auth middleware/resolvers) and plexams (business logic)
// packages can reach, without an import cycle. The auth middleware puts the resolved
// *model.User in the context with WithUser; any layer reads it with UserFromContext.
package principal

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
)

// contextKey is private so only this package can set/read the principal.
type contextKey string

const userContextKey contextKey = "authUser"

// WithUser returns a copy of ctx carrying the authenticated user.
func WithUser(ctx context.Context, user *model.User) context.Context {
	return context.WithValue(ctx, userContextKey, user)
}

// UserFromContext returns the authenticated user carried in ctx, or nil when none is
// present (e.g. a request that did not pass the auth middleware).
func UserFromContext(ctx context.Context) *model.User {
	user, _ := ctx.Value(userContextKey).(*model.User)
	return user
}
