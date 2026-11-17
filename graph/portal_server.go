package graph

import (
	"context"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/go-chi/chi/v5"
	portalgraph "github.com/obcode/plexams.go/graph/portal"
	portalgen "github.com/obcode/plexams.go/graph/portal/generated"
	"github.com/obcode/plexams.go/obs"
	"github.com/obcode/plexams.go/plexams/portal"
	"github.com/obcode/plexams.go/principal"
	"github.com/rs/zerolog/log"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// The examiner portal API is a second, small gqlgen server with its own schema
// (graph/portal). It is mounted twice:
//
//	/query/portal  a signed-in session; authMiddleware has resolved the principal
//	/query/token   a personal access token, Bearer only, no fallback
//
// None of the planner API's middlewares hangs on it -- no operation gate bound to the
// active semester, no root-field policy, no mutation log of the active semester -- and
// no planner type is reachable from its schema. What it allows is decided in
// plexams/portal.
const (
	portalPath = "/query/portal"
	tokenPath  = "/query/token"

	// portalMaxBody bounds a request. The portal's inputs are small forms.
	portalMaxBody = 1 << 20
	// portalComplexity bounds a query. Query fields resolve concurrently on the
	// connection pool the planners share.
	portalComplexity = 300
)

// newPortalServer builds the portal's GraphQL server.
func newPortalServer(service *portal.Service) *handler.Server {
	srv := handler.New(portalgen.NewExecutableSchema(portalgen.Config{
		Resolvers: portalgraph.NewResolver(service),
	}))
	// POST only: no websocket, no subscriptions, no GET with a query in the URL.
	srv.AddTransport(transport.POST{})
	srv.SetRecoverFunc(recoverFunc)
	// Introspection stays on, also in production: the schema is public in this
	// repository anyway, and it is what lets a colleague's tooling -- or Claude --
	// explore the API.
	srv.Use(extension.Introspection{})
	srv.Use(extension.FixedComplexityLimit(portalComplexity))

	// A token that may only read may not run a mutation. Structural, independent of
	// what any single resolver checks.
	srv.AroundOperations(func(ctx context.Context, next graphql.OperationHandler) graphql.ResponseHandler {
		oc := graphql.GetOperationContext(ctx)
		if oc.Operation != nil && isWriteOperation(oc) && !principal.From(ctx).MayWrite() {
			return graphql.OneShot(&graphql.Response{Errors: gqlerror.List{codedGQLError(portal.ErrReadOnlyToken)}})
		}
		return next(ctx)
	})

	srv.SetErrorPresenter(func(ctx context.Context, err error) *gqlerror.Error {
		if c, ok := portal.AsCoded(err); ok {
			e := codedGQLError(c)
			e.Path = graphql.GetPath(ctx)
			return e
		}
		return graphql.DefaultErrorPresenter(ctx, err)
	})
	return srv
}

func codedGQLError(c *portal.CodedError) *gqlerror.Error {
	return &gqlerror.Error{Message: c.Message, Extensions: map[string]any{"code": c.Code}}
}

// limitBody caps the request body.
func limitBody(max int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, max)
			next.ServeHTTP(w, r)
		})
	}
}

// requirePortalAccess lets a signed-in request through when the login has anything to
// do in the portal.
func requirePortalAccess(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !principal.From(r.Context()).MayUsePortal() {
			http.Error(w, "forbidden: user not authorized", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// tokenAuthenticator is the slice of the portal service the token door needs.
type tokenAuthenticator interface {
	Authenticate(ctx context.Context, presented string) (*principal.Principal, error)
}

// tokenAuth is the token door: Bearer only, and no fallback to X-Remote-User. A
// fallback would mean that a request reaching this mount with a forged header
// authenticates -- on the one route that, by design, has no proxy in front of it
// verifying the header.
func tokenAuth(auth tokenAuthenticator, limiter *rateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.allow("ip:" + clientIP(r)) {
				http.Error(w, "too many requests", http.StatusTooManyRequests)
				return
			}
			header := strings.TrimSpace(r.Header.Get("Authorization"))
			scheme, presented, ok := strings.Cut(header, " ")
			if !ok || !strings.EqualFold(scheme, "bearer") {
				w.Header().Set("WWW-Authenticate", `Bearer realm="plexams"`)
				http.Error(w, "unauthenticated: a Bearer token is required", http.StatusUnauthorized)
				return
			}
			pr, err := auth.Authenticate(r.Context(), presented)
			if err != nil {
				if _, coded := portal.AsCoded(err); !coded {
					log.Error().Err(err).Msg("cannot verify token")
					http.Error(w, "cannot verify token", http.StatusInternalServerError)
					return
				}
				w.Header().Set("WWW-Authenticate", `Bearer realm="plexams", error="invalid_token"`)
				http.Error(w, "unauthenticated: invalid token", http.StatusUnauthorized)
				return
			}
			if !limiter.allow("token:" + pr.Token.ID) {
				http.Error(w, "too many requests", http.StatusTooManyRequests)
				return
			}
			obs.SetUser(r.Context(), pr.Email)
			next.ServeHTTP(w, r.WithContext(principal.With(r.Context(), pr)))
		})
	}
}

// clientIP is the caller's address. Behind Caddy that is the first X-Forwarded-For
// entry, which Caddy sets itself (it does not trust a client's value); without one,
// the connection's address.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		first, _, _ := strings.Cut(xff, ",")
		return strings.TrimSpace(first)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// rateLimiter is a small token bucket per key: enough to keep one runaway script or
// one guessing client from occupying the connection pool the planners share. In
// memory; a restart resets it.
type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rate    float64 // tokens per second
	burst   float64
	now     func() time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

func newRateLimiter(perSecond, burst float64) *rateLimiter {
	return &rateLimiter{buckets: make(map[string]*bucket), rate: perSecond, burst: burst, now: time.Now}
}

func (l *rateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	b, ok := l.buckets[key]
	if !ok {
		if len(l.buckets) > 10000 {
			// Forget everything rather than grow without bound; the next requests
			// simply start with a full bucket.
			l.buckets = make(map[string]*bucket)
		}
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	b.tokens = min(l.burst, b.tokens+now.Sub(b.last).Seconds()*l.rate)
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// portalMount is what the router needs to serve the portal: nil when portal.enabled is
// off, and then neither route exists.
type portalMount struct {
	service *portal.Service
	// tokens mounts the token door. Off removes the route entirely -- an emergency stop
	// that leaves no code path that could be wrong about whether it is engaged.
	tokens bool
}

// mountPortal adds the two portal routes to the router.
func mountPortal(router interface {
	With(middlewares ...func(http.Handler) http.Handler) chi.Router
}, m *portalMount) {
	srv := newPortalServer(m.service)
	router.With(limitBody(portalMaxBody), requirePortalAccess).Handle(portalPath, srv)
	if m.tokens {
		limiter := newRateLimiter(10, 30)
		router.With(limitBody(portalMaxBody), tokenAuth(m.service, limiter)).Handle(tokenPath, srv)
	}
}
