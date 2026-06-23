package graph

import (
	"context"
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
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/obcode/plexams.go/graph/generated"
	"github.com/obcode/plexams.go/plexams"
	"github.com/rs/cors"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"github.com/vektah/gqlparser/v2/ast"
)

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

func StartServer(plexams *plexams.Plexams, port string) {
	plexamsResolver := NewResolver(plexams)

	origins := allowedOriginsFromConfig()
	originSet := make(map[string]bool, len(origins))
	for _, o := range origins {
		originSet[o] = true
	}

	c := generated.Config{Resolvers: plexamsResolver}
	srv := handler.New(generated.NewExecutableSchema(c))
	srv.AddTransport(transport.POST{})
	// Websocket transport carries GraphQL subscriptions (e.g. the streamed
	// output of long-running operations like invigilation generation).
	srv.AddTransport(transport.Websocket{
		KeepAlivePingInterval: 10 * time.Second,
		Upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return originSet[r.Header.Get("Origin")]
			},
		},
	})
	srv.Use(extension.Introspection{})

	// Block write mutations while any validation subscription is running, so the
	// GUI cannot mutate the plan underneath a running check.
	srv.AroundOperations(func(ctx context.Context, next graphql.OperationHandler) graphql.ResponseHandler {
		oc := graphql.GetOperationContext(ctx)
		if oc.Operation != nil && oc.Operation.Operation == ast.Mutation && !plexams.WritesAllowed() {
			return graphql.OneShot(graphql.ErrorResponse(ctx, "writes are blocked while a validation is running"))
		}
		return next(ctx)
	})

	// srv := handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{Resolvers: plexamsResolver}))

	router := chi.NewRouter()

	router.Use(cors.New(cors.Options{
		AllowedOrigins:   origins,
		AllowCredentials: true,
		Debug:            false,
	}).Handler)

	router.Handle("/", playground.Handler("GraphQL playground", "/query"))
	router.Handle("/query", srv)

	// Binary uploads (browser-generated PNGs, cover-page PDF ZIPs) for email
	// attachments; the send subscriptions read them back from the DB.
	router.Post("/upload/email-attachment", plexams.HTTPUploadEmailAttachment)
	router.Post("/upload/email-attachments-zip", plexams.HTTPUploadEmailAttachmentsZip)
	router.Get("/download/planned-rooms.json", plexams.HTTPDownloadPlannedRooms)

	server := &http.Server{Addr: fmt.Sprintf(":%s", port), Handler: router}
	defer server.Shutdown(context.Background()) // nolint:errcheck

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Startup failed")
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Info().Msg("Server will be shut down.")

	// log.Printf("connect to http://localhost:%s/ for GraphQL playground", port)
	// if err := http.ListenAndServe(":"+port, router); err != nil {
	// 	log.Fatal().Err(err).Msg("fatal error")
	// }
}
