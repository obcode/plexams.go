package graph

import (
	"context"
	"net/http"
	"strings"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// contextKey is a private type for context keys defined in this package, to avoid
// collisions with keys from other packages.
type contextKey string

const userContextKey contextKey = "authUser"

// UserFromContext returns the authenticated user carried in the context by
// authMiddleware, or nil when none is present (e.g. a request that did not pass the
// middleware). Resolvers use it to read the current identity/role.
func UserFromContext(ctx context.Context) *model.User {
	user, _ := ctx.Value(userContextKey).(*model.User)
	return user
}

// authProvider is the slice of *plexams.Plexams the auth middleware needs: the local
// dev identity (when auth is off) and the allow-list lookup (when it is on). Narrowed
// to an interface so the middleware is unit-testable without a database.
type authProvider interface {
	LocalDevUser() *model.User
	GetUserByEmail(ctx context.Context, email string) (*model.User, error)
}

// authMiddleware authenticates every HTTP request (GraphQL /query, the websocket
// upgrade, and all REST upload/download routes) by trusting the identity the auth
// proxy (Apache mod_shib or mod_auth_openidc) injects as a header, and authorizes it
// against the users allow-list. It injects the resolved *model.User into the request
// context (propagated to resolvers, incl. subscriptions via the websocket base ctx).
//
// Trust model: the backend does NOT authenticate itself — it trusts the proxy header
// and must therefore be reachable ONLY through the proxy (bind to loopback / not
// published), and the proxy must strip any client-sent value and set it authoritatively.
//
// When auth.enabled is false (the default → local development), it injects a
// full-access dev user so local operation is completely unchanged; no header is read
// and no request is ever rejected.
func authMiddleware(p authProvider) func(http.Handler) http.Handler {
	enabled := viper.GetBool("auth.enabled")

	header := strings.TrimSpace(viper.GetString("auth.header"))
	if header == "" {
		header = "X-Remote-User"
	}
	nameHeader := strings.TrimSpace(viper.GetString("auth.displaynameheader"))
	if nameHeader == "" {
		nameHeader = "X-Remote-Displayname"
	}

	if !enabled {
		log.Warn().Msg("auth is DISABLED (auth.enabled=false) — using local dev user, do NOT run like this on a server")
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var user *model.User

			if !enabled {
				user = p.LocalDevUser()
			} else {
				email := strings.ToLower(strings.TrimSpace(r.Header.Get(header)))
				if email == "" {
					http.Error(w, "unauthenticated: no identity from auth proxy", http.StatusUnauthorized)
					return
				}
				u, err := p.GetUserByEmail(r.Context(), email)
				if err != nil {
					http.Error(w, "cannot verify user", http.StatusInternalServerError)
					return
				}
				if u == nil {
					// fail-closed: only users on the allow-list get in
					log.Warn().Str("email", email).Msg("rejected login of unknown user")
					http.Error(w, "forbidden: user not authorized", http.StatusForbidden)
					return
				}
				if u.Name == "" {
					u.Name = strings.TrimSpace(r.Header.Get(nameHeader))
				}
				user = u
			}

			ctx := context.WithValue(r.Context(), userContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// roleCanWrite reports whether a role may perform data-changing operations. Today
// only PLANER writes; VIEWER is read-only. Extend here as finer-grained roles arrive.
func roleCanWrite(role model.Role) bool {
	return role == model.RolePlaner
}

// auditUser is the identity stamped onto the mutation_log: the authenticated user
// from the context (the real actor on the server), falling back to the local
// operator.* config when no principal is present (so nothing is lost if a write path
// bypasses the auth middleware).
func auditUser(ctx context.Context, p *plexams.Plexams) *string {
	if user := UserFromContext(ctx); user != nil && user.Email != "" {
		email := user.Email
		return &email
	}
	return p.OperatorID()
}
