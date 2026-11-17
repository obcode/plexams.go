package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	coderws "github.com/coder/websocket"
	sentryhttp "github.com/getsentry/sentry-go/http"
	"github.com/go-chi/chi/v5"
	"github.com/obcode/plexams.go/graph/generated"
	"github.com/obcode/plexams.go/obs"
	"github.com/obcode/plexams.go/plexams"
	"github.com/obcode/plexams.go/plexams/portal"
	"github.com/rs/cors"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"github.com/vektah/gqlparser/v2/ast"
)

// healthPath is the liveness endpoint. It is exempt from authentication
// (graph/auth.go) and, since 2026-08-23, IS routed to this backend by the reverse
// proxy (plexams.dev, deploy/Caddyfile) so that the deploy check and the uptime
// monitor can reach it. It answers a constant plus the build version; nothing
// about the semester, the database or a person passes through it.
const healthPath = "/healthz"

// defaultAllowedOrigins are the local dev frontends permitted for CORS and
// websocket upgrades when none are configured. plexams is a local single-user
// tool, so only localhost is allowed by default. Override via the config key
// server.allowedorigins (list of full origin URLs).
var defaultAllowedOrigins = []string{
	"http://localhost:5173",
	"http://localhost:8080",
	"http://localhost:3000",
}

// allowedOriginsFromConfig returns the configured CORS origins, or the defaults.
func allowedOriginsFromConfig() []string {
	if o := viper.GetStringSlice("server.allowedorigins"); len(o) > 0 {
		return o
	}
	return defaultAllowedOrigins
}

// originCheckedWebsocket gates the websocket upgrade on server.allowedorigins
// before handing the request to gqlgen's coder/websocket implementation.
//
// gqlgen 0.17.94 replaced the gorilla Upgrader (with its CheckOrigin callback)
// by a pluggable Implementation backed by coder/websocket. That library checks
// the origin itself, but only against the request host and without telling us
// which origin it rejected — so the check stays here, and the embedded
// implementation is set to skip its own (InsecureSkipVerify) rather than
// running a second, differently-configured one behind our back.
type originCheckedWebsocket struct {
	allowed map[string]bool
	origins []string
}

func (o originCheckedWebsocket) Accept(
	w http.ResponseWriter,
	r *http.Request,
	options transport.WebsocketAcceptOptions,
) (transport.WebsocketConn, error) {
	origin := r.Header.Get("Origin")
	// No Origin header → not a browser cross-origin request (same-origin, or a
	// server-side/proxied client behind the reverse proxy). Browsers always send
	// Origin on a cross-origin upgrade, so an empty Origin can never be a
	// cross-site websocket hijack — allow it.
	if origin != "" && !o.allowed[origin] {
		// Log the rejected origin: gqlgen does not surface it, so a config
		// mismatch (server.allowedorigins vs the real host) is otherwise a
		// silent "request origin not allowed" with no clue what to fix.
		log.Warn().Str("origin", origin).Strs("allowed", o.origins).
			Msg("websocket upgrade rejected: origin not in server.allowedorigins")
		return nil, fmt.Errorf("request origin not allowed")
	}
	return transport.CoderWebsocketImplementation{
		AcceptOptions: coderws.AcceptOptions{InsecureSkipVerify: true},
	}.Accept(w, r, options)
}

// gateState is the slice of *plexams.Plexams the operation gate needs: whether a
// validation or exclusive operation currently blocks writes, and whether the active
// semester is protected. Narrowed to an interface so the gate can be driven through
// the real executor in tests without a database.
type gateState interface {
	WritesAllowed() bool
	IsReadOnly() bool
}

// operationGate rejects operations before any field runs: a read-only role may not
// change data, writes are blocked while a validation is running, and a protected
// semester accepts no data-changing operation.
func operationGate(state gateState) graphql.OperationMiddleware {
	return func(ctx context.Context, next graphql.OperationHandler) graphql.ResponseHandler {
		oc := graphql.GetOperationContext(ctx)
		if oc.Operation != nil {
			op := oc.Operation.Operation
			// Authorization: a read-only role (VIEWER) may query and run validations
			// but must not perform any data-changing operation. Enforced in the backend
			// — the GUI is never the security boundary.
			// No principal means the request did not pass the auth middleware: fail
			// closed instead of skipping the role check.
			user := UserFromContext(ctx)
			if user == nil {
				return graphql.OneShot(graphql.ErrorResponse(ctx, "forbidden: no authenticated user"))
			}
			if !roleCanWrite(user.Role) && isWriteOperation(oc) {
				return graphql.OneShot(graphql.ErrorResponse(ctx, "forbidden: your role is read-only"))
			}
			if op == ast.Mutation && !state.WritesAllowed() && !isTodoOnlyOperation(oc) {
				return graphql.OneShot(graphql.ErrorResponse(ctx, "writes are blocked while a validation is running"))
			}
			// read-only database: reject data-changing operations, but allow queries,
			// validations, switching the semester, toggling read-only and todos.
			if state.IsReadOnly() && isDataChangingOperation(oc) && !isTodoOnlyOperation(oc) {
				return graphql.OneShot(graphql.ErrorResponse(ctx, "semester is read-only"))
			}
		}
		return next(ctx)
	}
}

// newGraphQLServer builds the gqlgen server with its transports, the recover function,
// introspection (outside production) and the operation gate. It is separate from
// StartServer so tests can run operations through the real executor.
func newGraphQLServer(es graphql.ExecutableSchema, state gateState, origins []string, production bool) *handler.Server {
	originSet := make(map[string]bool, len(origins))
	for _, o := range origins {
		originSet[o] = true
	}

	srv := handler.New(es)
	srv.AddTransport(transport.POST{})
	// Websocket transport carries GraphQL subscriptions (e.g. the streamed
	// output of long-running operations like invigilation generation).
	srv.AddTransport(transport.Websocket{
		KeepAlivePingInterval: 10 * time.Second,
		Implementation:        originCheckedWebsocket{allowed: originSet, origins: origins},
	})
	srv.SetRecoverFunc(recoverFunc)

	// In production the GraphQL introspection is turned off (server.production=true);
	// locally it stays on for the playground and tooling.
	if !production {
		srv.Use(extension.Introspection{})
	}

	// Block write mutations while any validation subscription is running, so the
	// GUI cannot mutate the plan underneath a running check.
	srv.AroundOperations(operationGate(state))

	// Field by field: the least role each root field needs (graph/policy.go).
	// Registered here, before StartServer adds the logging and dirty-marking field
	// middlewares, so it runs outermost and a denied call reaches none of them.
	srv.AroundFields(rootFieldPolicyMiddleware(rootFieldPolicy))

	return srv
}

// restRoutes are the REST upload/download handlers mounted next to /query. Collected
// in a struct so the router can be built with stubs in tests.
type restRoutes struct {
	uploadEmailAttachment     http.HandlerFunc
	uploadEmailAttachmentsZip http.HandlerFunc
	uploadPrimussZip          http.HandlerFunc
	uploadJiraAttachment      http.HandlerFunc
	uploadDatasetCSV          http.HandlerFunc
	downloadPlannedRooms      http.HandlerFunc
	downloadPDF               http.HandlerFunc
	downloadCSVDraft          http.HandlerFunc
	downloadICS               http.HandlerFunc
	downloadDatasetCSV        http.HandlerFunc
	downloadMyInputsCSV       http.HandlerFunc
}

// restRoutesOf binds the REST routes to the real handlers.
func restRoutesOf(p *plexams.Plexams) restRoutes {
	return restRoutes{
		uploadEmailAttachment:     p.HTTPUploadEmailAttachment,
		uploadEmailAttachmentsZip: p.HTTPUploadEmailAttachmentsZip,
		uploadPrimussZip:          p.HTTPUploadPrimussZip,
		uploadJiraAttachment:      p.HTTPUploadJiraAttachment,
		uploadDatasetCSV:          p.HTTPUploadDatasetCSV,
		downloadPlannedRooms:      p.HTTPDownloadPlannedRooms,
		downloadPDF:               p.HTTPDownloadPDF,
		downloadCSVDraft:          p.HTTPDownloadCSVDraft,
		downloadICS:               p.HTTPDownloadICS,
		downloadDatasetCSV:        p.HTTPDownloadDatasetCSV,
		downloadMyInputsCSV:       p.HTTPDownloadMyInputsCSV,
	}
}

// newRouter mounts the middleware chain, /query, the health check and the REST routes.
// It is separate from StartServer so the chain can be exercised with httptest.
func newRouter(auth authProvider, state gateState, graphQL http.Handler, rest restRoutes, origins []string, production bool, portalRoutes *portalMount) chi.Router {
	router := chi.NewRouter()

	// FIRST, before everything else. It clones a Sentry hub onto every request
	// context, which is what lets a panic report carry that request's user
	// (graph/auth.go), its breadcrumbs (graph/mutation_logging.go) and its
	// request data instead of the process-wide defaults.
	//
	// Repanic is on so it never swallows anything: the recovery belongs to
	// recoverMiddleware right below, which also answers with a 500. A no-op
	// when reporting is disabled.
	router.Use(sentryhttp.New(sentryhttp.Options{Repanic: true}).Handle)

	// Turn a panic anywhere below into a logged, reported 500 -- the REST
	// upload/download routes have no protection of their own (see recover.go).
	router.Use(recoverMiddleware)

	router.Use(cors.New(cors.Options{
		AllowedOrigins:   origins,
		AllowCredentials: true,
		Debug:            false,
	}).Handler)

	// Authenticate/authorize every request (GraphQL, websocket upgrade, REST routes)
	// from the auth-proxy header, or inject a local dev user when auth is disabled.
	// Runs after CORS so preflight OPTIONS are short-circuited before reaching it.
	router.Use(authMiddleware(auth))

	// The GraphQL playground is a dev convenience; disabled in production (the GUI is
	// served by the reverse proxy there, not by this backend).
	// The planner API: a login without a planner role -- a portal-only login -- gets
	// the same 403 as an unknown one. The operation gate would refuse it too, but with a
	// 200 and a GraphQL error; the route is the clearer boundary.
	if !production {
		router.With(requireStaffRole).Handle("/", playground.Handler("GraphQL playground", "/query"))
	}
	router.With(requireStaffRole).Handle("/query", graphQL)

	// The examiner portal: only when portal.enabled is set (graph/portal_server.go).
	if portalRoutes != nil {
		mountPortal(router, portalRoutes)
	}

	// Liveness for the container health check, which is what makes a deploy fail
	// loudly instead of quietly: the server migrates the schema at startup and
	// exits on a migration error, `restart: unless-stopped` turns that into a
	// crash loop, and `docker compose up -d` alone would still report success.
	//
	// Deliberately unauthenticated (see authMiddleware) and deliberately NOT a
	// readiness check: the process is only listening once the database is up, so
	// there is nothing left to probe that a 200 does not already say.
	//
	// It reports the running version, and that is not decoration. This server ran
	// SEVENTEEN DAYS on a stale image once, because a deploy that never started is
	// invisible: nothing turns red, the old container keeps answering, and every
	// check that only asks "does something answer" says yes. Since the reverse
	// proxy routes this path (plexams.dev, deploy/Caddyfile), the deploy job and
	// the uptime monitor can ask WHICH build is answering instead.
	//
	// NOTE for anyone comparing the value: goreleaser sets it WITHOUT a leading
	// "v" ("5.2.1"), while the deployed image tag has one ("v5.2.1"). Normalise
	// both sides before comparing — see releaseURL() below, which exists for the
	// same reason.
	router.Get(healthPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// A cached liveness answer is a lie waiting to happen.
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "ok",
			"version": viper.GetString("Version"),
			"commit":  viper.GetString("Commit"),
		})
	})

	// Uploads change data: a writing role, an allowed origin and -- where they write
	// into the active semester -- an unprotected semester (rest_authz.go).
	router.Group(func(r chi.Router) {
		r.Use(requireWriter(state, origins, true))
		// Binary uploads (browser-generated PNGs, cover-page PDF ZIPs) for email
		// attachments; the send subscriptions read them back from the DB.
		r.Post("/upload/email-attachment", rest.uploadEmailAttachment)
		r.Post("/upload/email-attachments-zip", rest.uploadEmailAttachmentsZip)
		r.Post("/upload/primuss-zip", rest.uploadPrimussZip)
		r.Post("/upload/dataset-csv", rest.uploadDatasetCSV)
	})
	router.Group(func(r chi.Router) {
		// Attaching a file to a Jira issue changes nothing in the semester.
		r.Use(requireWriter(state, origins, false))
		r.Post("/upload/jira-attachment", rest.uploadJiraAttachment)
	})

	// Downloads reveal planning data: any role on the allow-list.
	router.Group(func(r chi.Router) {
		r.Use(requireStaffRole)
		r.Get("/download/planned-rooms.json", rest.downloadPlannedRooms)

		// Generated documents (formerly the pdf/csv/ics CLI commands): draft plans and
		// exports for the faculties/examers.
		r.Get("/download/pdf/{kind}", rest.downloadPDF)
		r.Get("/download/csv/{kind}", rest.downloadCSVDraft)
		r.Get("/download/ics/{program}", rest.downloadICS)

		// Human-readable CSV of the manually entered data (absolute date/time, robust
		// against exam-period shifts): per-dataset and a combined "my inputs" ZIP.
		r.Get("/download/dataset-csv", rest.downloadDatasetCSV)
		r.Get("/download/my-inputs-csv.zip", rest.downloadMyInputsCSV)
	})

	return router
}

func StartServer(plexams *plexams.Plexams, port string) {
	plexamsResolver := NewResolver(plexams)

	origins := allowedOriginsFromConfig()
	production := viper.GetBool("server.production")

	c := generated.Config{Resolvers: plexamsResolver}
	srv := newGraphQLServer(generated.NewExecutableSchema(c), plexams, origins, production)

	// Log every mutating operation (mutations + data-changing subscriptions) to the
	// per-semester mutation_log collection.
	srv.AroundFields(mutationLogMiddleware(plexams))

	// Mark the cached assembled exams stale when an input changes (for the GUI banner).
	srv.AroundFields(assembledExamsDirtyMiddleware(plexams))

	// Mark the prepared student regs stale when an input changes (for the GUI banner).
	srv.AroundFields(studentRegsDirtyMiddleware(plexams))

	var portalRoutes *portalMount
	if viper.GetBool("portal.enabled") {
		portalRoutes = &portalMount{
			service:   portal.New(plexams.PortalStore()),
			inquiries: plexams.Inquiries(),
			tokens:    viper.GetBool("portal.tokens"),
		}
		log.Info().Bool("tokens", portalRoutes.tokens).Msg("examiner portal enabled")
	}

	router := newRouter(plexams, plexams, srv, restRoutesOf(plexams), origins, production, portalRoutes)

	server := &http.Server{Addr: fmt.Sprintf(":%s", port), Handler: router}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	// The nightly auto-sync scheduler shares the server lifetime. Its context is a
	// fallback stop; the primary, graceful stop is sched.Shutdown below, which drains an
	// in-flight run instead of cutting it off.
	schedulerCtx, cancelScheduler := context.WithCancel(context.Background())
	defer cancelScheduler()
	sched := startScheduledSync(schedulerCtx, plexams)
	// A second, independent scheduler for the daily admin-overview digest (nil when disabled).
	digestSched := startAdminDigestMail(schedulerCtx, plexams)
	// The planners' notification about portal changes (nil without the portal).
	changeSched := startPortalNotifications(schedulerCtx, plexams)

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Startup failed")
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Info().Msg("Server will be shut down.")
	if sched != nil {
		// Drain a running auto-sync gracefully (bounded); on timeout it is cancelled
		// cooperatively. A cut-off run is safe: the sync is idempotent and self-heals
		// via the next nightly run plus the startup catch-up.
		drainCtx, cancelDrain := context.WithTimeout(context.Background(), 30*time.Second)
		if err := sched.Shutdown(drainCtx); err != nil {
			log.Warn().Err(err).Msg("auto-sync did not drain within grace period")
		}
		cancelDrain()
	}
	if digestSched != nil {
		// Drain a running digest send gracefully; on timeout it is cancelled cooperatively.
		drainCtx, cancelDrain := context.WithTimeout(context.Background(), 30*time.Second)
		if err := digestSched.Shutdown(drainCtx); err != nil {
			log.Warn().Err(err).Msg("admin digest did not drain within grace period")
		}
		cancelDrain()
	}
	if changeSched != nil {
		drainCtx, cancelDrain := context.WithTimeout(context.Background(), 30*time.Second)
		if err := changeSched.Shutdown(drainCtx); err != nil {
			log.Warn().Err(err).Msg("portal daily mail did not drain within grace period")
		}
		cancelDrain()
	}
	cancelScheduler()

	// Last, and after the drains: an error raised while shutting down is
	// exactly the kind that never gets noticed otherwise, and it can only be
	// reported if the queue is flushed after it was queued.
	obs.Flush()

	// log.Printf("connect to http://localhost:%s/ for GraphQL playground", port)
	// if err := http.ListenAndServe(":"+port, router); err != nil {
	// 	log.Fatal().Err(err).Msg("fatal error")
	// }
}
