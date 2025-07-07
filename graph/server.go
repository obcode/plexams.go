package graph

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/go-chi/chi/v5"
	"github.com/obcode/plexams.go/graph/generated"
	"github.com/obcode/plexams.go/plexams"
	"github.com/rs/cors"
	"github.com/rs/zerolog/log"
)

func StartServer(plexams *plexams.Plexams, port string) {
	plexamsResolver := NewResolver(plexams)

	c := generated.Config{Resolvers: plexamsResolver}
	srv := handler.New(generated.NewExecutableSchema(c))
	srv.AddTransport(transport.POST{})
	srv.Use(extension.Introspection{})

	// srv := handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{Resolvers: plexamsResolver}))

	router := chi.NewRouter()

	router.Use(cors.New(cors.Options{
		AllowedOrigins:   []string{"http://localhost:5173", "http://localhost:8080", "http://localhost:3000"},
		AllowCredentials: true,
		Debug:            false,
	}).Handler)

	router.Handle("/", playground.Handler("GraphQL playground", "/query"))
	router.Handle("/query", srv)

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
