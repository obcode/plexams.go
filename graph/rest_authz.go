package graph

import (
	"net/http"
	"net/url"
	"strings"
)

// The REST upload and download routes sit next to /query but bypass the GraphQL
// operation gate, so they need their own checks. authMiddleware has already
// rejected unknown logins; these add the role, the protection of the active
// semester and a same-origin check for the state-changing uploads.

// requireStaffRole lets a request through when it carries a principal with any
// role. A download reveals planning data, so it needs a login on the allow-list
// -- which authMiddleware guarantees today -- and fails closed if one is missing.
func requireStaffRole(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if UserFromContext(r.Context()) == nil {
			http.Error(w, "forbidden: no authenticated user", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireWriter guards an upload: it changes data, so it needs a writing role,
// and a browser request must come from an allowed origin. Uploads are multipart
// form posts, which a foreign page can send without a CORS preflight; the session
// cookie would ride along. Checking Origin closes that.
//
// semesterData marks uploads that write into the active semester; those are also
// refused while the semester is protected.
func requireWriter(state gateState, allowedOrigins []string, semesterData bool) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[strings.TrimRight(o, "/")] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := UserFromContext(r.Context())
			if user == nil {
				http.Error(w, "forbidden: no authenticated user", http.StatusForbidden)
				return
			}
			if !roleCanWrite(user.Role) {
				http.Error(w, "Dein Zugang darf nur lesen (Rolle VIEWER).", http.StatusForbidden)
				return
			}
			if !sameOrAllowedOrigin(r, allowed) {
				http.Error(w, "forbidden: cross-origin upload", http.StatusForbidden)
				return
			}
			if semesterData && state.IsReadOnly() {
				http.Error(w, "Das Semester ist geschützt (nur lesen); es wurde nichts importiert.", http.StatusLocked)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// sameOrAllowedOrigin accepts a request without an Origin header (not a browser
// cross-site request), one from the request's own host, or one from a configured
// origin (the dev GUI on another port).
func sameOrAllowedOrigin(r *http.Request, allowed map[string]bool) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	if allowed[strings.TrimRight(origin, "/")] {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return u.Host != "" && strings.EqualFold(u.Host, r.Host)
}
