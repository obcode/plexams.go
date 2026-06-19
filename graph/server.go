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
	"github.com/vektah/gqlparser/v2/ast"
)

// allowedOrigins are the local dev frontends permitted for CORS and websocket
// upgrades. plexams is a local single-user tool, so only localhost is allowed.
var allowedOrigins = map[string]bool{
	"http://localhost:5173": true,
	"http://localhost:8080": true,
	"http://localhost:3000": true,
}

func StartServer(plexams *plexams.Plexams, port string) {
	plexamsResolver := NewResolver(plexams)

	c := generated.Config{Resolvers: plexamsResolver}
	srv := handler.New(generated.NewExecutableSchema(c))
	srv.AddTransport(transport.POST{})
	// Websocket transport carries GraphQL subscriptions (e.g. the streamed
	// output of long-running operations like invigilation generation).
	srv.AddTransport(transport.Websocket{
		KeepAlivePingInterval: 10 * time.Second,
		Upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return allowedOrigins[r.Header.Get("Origin")]
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

	origins := make([]string, 0, len(allowedOrigins))
	for o := range allowedOrigins {
		origins = append(origins, o)
	}
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
