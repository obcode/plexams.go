package graph

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/obs"
	"github.com/obcode/plexams.go/plexams"
	"github.com/obcode/plexams.go/principal"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// UserFromContext returns the authenticated user carried in the context by
// authMiddleware, or nil when none is present (e.g. a request that did not pass the
// middleware). Resolvers use it to read the current identity/role. The context value
// lives in the neutral principal package so plexams can read it too.
func UserFromContext(ctx context.Context) *model.User {
	return principal.UserFromContext(ctx)
}

// authProvider is the slice of *plexams.Plexams the auth middleware needs: the local
// dev identity (when auth is off) and the allow-list lookup (when it is on). Narrowed
// to an interface so the middleware is unit-testable without a database.
type authProvider interface {
	LocalDevUser() *model.User
	GetUserByEmail(ctx context.Context, email string) (*model.User, error)
	// ResolvePortalLogin returns what the portal tables say about a login. Only asked
	// when portal.enabled is set.
	ResolvePortalLogin(ctx context.Context, email string) (*db.PortalLogin, error)
}

// authMiddleware authenticates every HTTP request (GraphQL /query, the websocket
// upgrade, and all REST upload/download routes) by trusting the identity the auth
// proxy (nginx + oauth2-proxy, OIDC) injects as a header, and authorizes it
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

	portalEnabled := viper.GetBool("portal.enabled")
	affiliationHeader := strings.TrimSpace(viper.GetString("auth.affiliationheader"))
	if affiliationHeader == "" {
		affiliationHeader = "X-Remote-Affiliation"
	}
	denyStudents := viper.GetBool("portal.denystudents")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// The container health check runs INSIDE the container and carries no
			// identity header -- fail-closed would answer it with 401, and Docker
			// would call a perfectly healthy server unhealthy. It is exempt here
			// rather than mounted before this middleware because chi requires all
			// middleware to be declared before any route.
			//
			// Nothing leaks by it: the handler answers a constant plus the build
			// version. It IS routed from outside since 2026-08-23 (plexams.dev,
			// deploy/Caddyfile) so the deploy check and the uptime monitor can ask
			// which build is answering -- this server once ran seventeen days on a
			// stale image without anything noticing.
			if r.URL.Path == healthPath {
				next.ServeHTTP(w, r)
				return
			}

			var pr *principal.Principal

			if !enabled {
				pr = devPrincipal(r.Context(), p, portalEnabled)
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
				pr = &principal.Principal{Email: email, Staff: u}
				if u != nil {
					pr.Name = u.Name
				}

				if portalEnabled {
					login, err := p.ResolvePortalLogin(r.Context(), email)
					if err != nil {
						http.Error(w, "cannot verify user", http.StatusInternalServerError)
						return
					}
					applyPortalLogin(pr, login)
					// Second lock against students: they are no ZPA persons, so the match
					// above already keeps them out; this covers an address that matches
					// anyway. Only when switched on -- the claim values have to be
					// verified first.
					if denyStudents && studentOnly(r.Header.Get(affiliationHeader)) {
						pr.TeacherIDs, pr.ActsFor, pr.Grants = nil, nil, nil
					}
				}

				if pr.Staff == nil && !pr.MayUsePortal() {
					// fail-closed: a login needs a planner role or something to do in the
					// portal
					log.Warn().Str("email", email).Msg("rejected login of unknown user")
					rejectedLogins.add(email)
					http.Error(w, "forbidden: user not authorized", http.StatusForbidden)
					return
				}
				if pr.Name == "" {
					pr.Name = strings.TrimSpace(r.Header.Get(nameHeader))
				}
				if pr.Staff != nil && pr.Staff.Name == "" {
					pr.Staff.Name = pr.Name
				}
			}

			// Attach the actor to this request's error reports -- as a keyed
			// pseudonym, so "how many people are affected" is answerable
			// without the address leaving the host (obs/user.go).
			obs.SetUser(r.Context(), pr.Email)

			ctx := principal.With(r.Context(), pr)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Roles form a hierarchy ADMIN ⊇ PLANER ⊇ VIEWER (a user has exactly one role).

// roleCanWrite reports whether a role may perform data-changing operations: PLANER
// and ADMIN (which includes all PLANER rights); VIEWER is read-only.
func roleCanWrite(role model.Role) bool {
	return role == model.RolePlaner || role == model.RoleAdmin
}

// roleCanAdmin reports whether a role may administer users (setUser/removeUser).
func roleCanAdmin(role model.Role) bool {
	return role == model.RoleAdmin
}

// requireAdmin returns an error unless the request's principal is an ADMIN. Used to
// gate the user-administration mutations in the backend (the security boundary).
func requireAdmin(ctx context.Context) error {
	user := UserFromContext(ctx)
	if user == nil || !roleCanAdmin(user.Role) {
		return fmt.Errorf("forbidden: user administration requires role ADMIN")
	}
	return nil
}

// auditUser is the identity stamped onto the mutation_log: the authenticated user
// from the context (the real actor on the server), falling back to the local
// operator.* config when no principal is present (so nothing is lost if a write path
// bypasses the auth middleware).
func auditUser(ctx context.Context, p *plexams.Plexams) *string {
	if pr := principal.From(ctx); pr != nil && pr.Email != "" {
		email := pr.Email
		return &email
	}
	return p.OperatorID()
}

// applyPortalLogin copies what the portal tables say onto the principal.
func applyPortalLogin(pr *principal.Principal, login *db.PortalLogin) {
	if login == nil {
		return
	}
	pr.TeacherIDs = login.TeacherIDs
	pr.ActsFor = login.ActsFor
	pr.Blocked = login.Blocked
	if len(login.Grants) > 0 {
		pr.Grants = make(map[string]principal.GrantLevel, len(login.Grants))
		for program, level := range login.Grants {
			pr.Grants[program] = principal.GrantLevel(level)
		}
	}
	if pr.Name == "" && len(login.TeacherIDs) > 0 {
		pr.Name = login.TeacherNames[login.TeacherIDs[0]]
	}
}

// studentOnly reports whether an eduPersonScopedAffiliation value names students and
// nothing else. oauth2-proxy passes the claim as a comma-separated list such as
// "student@hm.edu" or "employee@hm.edu,member@hm.edu". Empty is not "student only":
// without the claim nobody is turned away by it.
func studentOnly(value string) bool {
	seen := false
	for _, part := range strings.Split(value, ",") {
		part = strings.ToLower(strings.TrimSpace(part))
		if part == "" {
			continue
		}
		role, _, _ := strings.Cut(part, "@")
		if role != "student" {
			return false
		}
		seen = true
	}
	return seen
}

// devPrincipal is the identity used when auth is disabled (local development). By
// default the full-access ADMIN dev user, as before. auth.devrole changes the planner
// role (ADMIN, PLANER, VIEWER or NONE) and auth.devteacherids makes the dev login a ZPA
// person, so both sides of the access model can be exercised locally.
func devPrincipal(ctx context.Context, p authProvider, portalEnabled bool) *principal.Principal {
	user := p.LocalDevUser()
	pr := &principal.Principal{Email: user.Email, Name: user.Name, Staff: user}

	switch role := strings.ToUpper(strings.TrimSpace(viper.GetString("auth.devrole"))); role {
	case "":
	case "NONE":
		pr.Staff = nil
	default:
		if r := model.Role(role); r.IsValid() {
			devUser := *user
			devUser.Role = r
			pr.Staff = &devUser
		} else {
			log.Warn().Str("role", role).Msg("ignoring unknown auth.devrole")
		}
	}

	if portalEnabled {
		if login, err := p.ResolvePortalLogin(ctx, pr.Email); err == nil {
			applyPortalLogin(pr, login)
		}
		for _, id := range viper.GetIntSlice("auth.devteacherids") {
			if id > 0 && !slices.Contains(pr.TeacherIDs, id) {
				pr.TeacherIDs = append(pr.TeacherIDs, id)
			}
		}
	}
	return pr
}
