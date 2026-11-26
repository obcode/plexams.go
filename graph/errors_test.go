package graph

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/apierr"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

func TestClientErrorKeepsCodeAndHidesDatabaseDetails(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantCode    string
		wantMessage string
	}{
		{"coded", apierr.New(apierr.CodeBusy, "busy"), apierr.CodeBusy, "busy"},
		{"coded, wrapped", fmt.Errorf("sending: %w", apierr.New(apierr.CodeAlreadySent, "sent")),
			apierr.CodeAlreadySent, "sending: sent"},
		{"missing row", fmt.Errorf("exam 7: %w", pgx.ErrNoRows), apierr.CodeInternal, internalMessage},
		{"server-side database error", &pgconn.PgError{Code: "23505", ConstraintName: "exam_pkey"},
			apierr.CodeInternal, internalMessage},
		{"plain domain error", fmt.Errorf("room R1 has only 3 seats"), "", "room R1 has only 3 seats"},
		{"wrapped by gqlgen with position and path",
			gqlerror.WrapPath(ast.Path{ast.PathName("createSemester")}, apierr.New(apierr.CodeInvalidInput, "Startzeit fehlt.")),
			apierr.CodeInvalidInput, "Startzeit fehlt."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message, code := clientError(tt.err)
			if code != tt.wantCode || message != tt.wantMessage {
				t.Errorf("clientError() = (%q, %q), want (%q, %q)", message, code, tt.wantMessage, tt.wantCode)
			}
		})
	}
}

func TestEmitErrorCarriesTheCode(t *testing.T) {
	ch := make(chan *model.LogLine, 2)
	reporter := newStreamReporter(context.Background(), ch)

	reporter.emitError(errBusy)
	reporter.emitError(fmt.Errorf("plain"))

	coded, plain := <-ch, <-ch
	if coded.Level != model.LogLevelError || coded.Code == nil || *coded.Code != apierr.CodeBusy {
		t.Errorf("coded line = %+v, want an ERROR line with code %s", coded, apierr.CodeBusy)
	}
	if plain.Code != nil || plain.Text != "Fehler: plain" {
		t.Errorf("plain line = %+v, want text %q without a code", plain, "Fehler: plain")
	}
}
