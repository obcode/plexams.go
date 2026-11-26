package graph

import (
	"context"
	"errors"
	"strings"

	"github.com/99designs/gqlgen/graphql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/apierr"
	"github.com/rs/zerolog/log"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// The two refusals of a stream that cannot start. Coded, so the GUI tells "busy"
// from a failure without reading the text.
var (
	errBusy           = apierr.New(apierr.CodeBusy, "Gerade läuft eine Validierung, ein Import, eine Übertragung oder ein E-Mail-Versand; der Vorgang kann erst danach starten.")
	errValidationBusy = apierr.New(apierr.CodeBusy, "Gerade läuft ein Import, eine Übertragung oder ein E-Mail-Versand; die Validierung kann erst danach starten.")
)

// internalMessage is all a client learns about a fault it cannot do anything about.
const internalMessage = "Interner Fehler in Plexams. Das liegt nicht an der Eingabe; bitte noch einmal versuchen."

// isInternal reports whether err is a database fault rather than something the
// planner did: a missing row nobody translated, or an error the server itself
// reported. Their texts ("no rows in result set", "SQLSTATE 23505") name tables and
// constraints and help nobody in front of the GUI.
func isInternal(err error) bool {
	var pgErr *pgconn.PgError
	return errors.Is(err, pgx.ErrNoRows) || errors.As(err, &pgErr)
}

// clientError reduces err to what a client may see: the message and, where known,
// a code. A database fault keeps its details in the log and is reported as INTERNAL.
func clientError(err error) (message, code string) {
	// gqlgen wraps a resolver's error with its position and path, and Error() of
	// the wrapper prints them ("input:3:5: createSemester ..."). The planner only
	// needs the error itself.
	var gqlErr *gqlerror.Error
	if errors.As(err, &gqlErr) && gqlErr.Err != nil {
		err = gqlErr.Err
	}
	if c, ok := apierr.As(err); ok {
		return err.Error(), c.Code
	}
	if isInternal(err) {
		log.Error().Err(err).Msg("database error reached the API")
		return internalMessage, apierr.CodeInternal
	}
	return err.Error(), ""
}

// presentError is the error presenter of the planning API: it puts the code of a
// coded error into extensions.code and keeps database details away from the client.
func presentError(ctx context.Context, err error) *gqlerror.Error {
	// An error that already carries extensions was built on purpose (the recover
	// function, the operation gate); leave it alone.
	var gqlErr *gqlerror.Error
	if errors.As(err, &gqlErr) && gqlErr.Extensions != nil {
		return graphql.DefaultErrorPresenter(ctx, err)
	}
	message, code := clientError(err)
	presented := graphql.DefaultErrorPresenter(ctx, err)
	presented.Message = message
	if code != "" {
		presented.Extensions = map[string]any{"code": code}
	}
	return presented
}

// codedResponse answers an operation with a single coded error, before any field ran.
func codedResponse(code, message string) graphql.ResponseHandler {
	return graphql.OneShot(&graphql.Response{Errors: gqlerror.List{{
		Message:    message,
		Extensions: map[string]any{"code": code},
	}}})
}

// emitError streams err as an ERROR line, with its code where it has one.
func (r *streamReporter) emitError(err error) {
	message, code := clientError(err)
	line := &model.LogLine{Level: model.LogLevelError, Text: strings.TrimSuffix("Fehler: "+message, "\n")}
	if code != "" {
		line.Code = &code
	}
	r.send(line)
}
