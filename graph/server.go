package graph

import (
	"net/http"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/go-chi/chi"
	"github.com/obcode/plexams.go/graph/generated"
	"github.com/obcode/plexams.go/plexams"
	"github.com/rs/cors"
	"github.com/rs/zerolog/log"
)

func StartServer(plexams *plexams.Plexams, port string) {
	plexamsResolver := NewResolver(plexams)

	srv := handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{Resolvers: plexamsResolver}))

	router := chi.NewRouter()

	router.Use(cors.New(cors.Options{
		AllowedOrigins:   []string{"http://localhost:5173", "http://localhost:8080"},
		AllowCredentials: true,
		Debug:            false,
	}).Handler)

	router.Handle("/", playground.Handler("GraphQL playground", "/query"))
	router.Handle("/query", srv)

	log.Printf("connect to http://localhost:%s/ for GraphQL playground", port)
	if err := http.ListenAndServe(":"+port, router); err != nil {
		log.Fatal().Err(err).Msg("fatal error")
	}
}
