package graph

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/obcode/plexams.go/obs"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// captureLog swaps the global logger for a JSON one writing into a buffer, and
// restores it afterwards. The recover path logs through the package-level logger
// like the rest of the server, so this is the only way to see what it produced.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	saved := log.Logger
	log.Logger = zerolog.New(&buf)
	t.Cleanup(func() { log.Logger = saved })
	return &buf
}

func decode(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatalf("log line is not JSON: %v -- %q", err, buf.String())
	}
	return entry
}

// The whole point of replacing DefaultRecover: the panic must arrive in the
// logger, not on raw stderr.
func TestRecoverFuncLogsThroughZerolog(t *testing.T) {
	buf := captureLog(t)

	err := recoverFunc(context.Background(), "boom")
	if err == nil {
		t.Fatal("recoverFunc must return an error for the client")
	}
	if got := err.Error(); !strings.Contains(got, "internal system error") {
		t.Errorf("client message changed, was %q -- DefaultRecover's wording is deliberate", got)
	}

	entry := decode(t, buf)
	if entry["level"] != "error" {
		t.Errorf("level = %v, want error", entry["level"])
	}
	if entry["panic"] != "boom" {
		t.Errorf("panic = %v, want boom", entry["panic"])
	}
	if s, _ := entry["stack"].(string); !strings.Contains(s, "recover_test.go") {
		t.Errorf("stack does not point at the caller: %q", s)
	}
}

// A panic during parsing or validation happens before the operation context
// exists. GetOperationContext panics in that case, which would turn a reported
// bug into an unreported one -- hence the HasOperationContext guard.
func TestRecoverFuncWithoutOperationContext(t *testing.T) {
	buf := captureLog(t)

	if graphql.HasOperationContext(context.Background()) {
		t.Fatal("precondition: a bare context must not carry an operation context")
	}
	// Must not panic.
	_ = recoverFunc(context.Background(), "boom")

	if _, ok := decode(t, buf)["operation"]; ok {
		t.Error("operation must be absent when there is no operation context")
	}
}

func TestRecoverFuncRecordsOperationName(t *testing.T) {
	buf := captureLog(t)

	ctx := graphql.WithOperationContext(context.Background(), &graphql.OperationContext{
		OperationName: "PlanExam",
		RawQuery:      `mutation PlanExam { nta(mtknr: "12345678") { ok } }`,
		Variables:     map[string]any{"mtknr": "12345678"},
	})
	_ = recoverFunc(ctx, "boom")

	entry := decode(t, buf)
	if entry["operation"] != "PlanExam" {
		t.Errorf("operation = %v, want PlanExam", entry["operation"])
	}
	// The query document and the variables may carry a Matrikelnummer; these logs
	// are meant to be shippable, so neither may appear anywhere in the entry.
	if strings.Contains(buf.String(), "12345678") {
		t.Errorf("student data leaked into the log entry: %s", buf.String())
	}
}

// The log line must be marked so the error tracker ignores it: obs.CapturePanic
// has already reported this panic with a stack that reaches the resolver, and
// the log path would send a second copy showing zerolog's internals instead.
func TestRecoverFuncKeepsItsLogLineOutOfTheErrorReport(t *testing.T) {
	buf := captureLog(t)

	_ = recoverFunc(context.Background(), "boom")

	if entry := decode(t, buf); entry[obs.SkipField] != true {
		t.Errorf("%s = %v, want true", obs.SkipField, entry[obs.SkipField])
	}
}

func TestRecoverMiddlewareAnswersWithFiveHundred(t *testing.T) {
	buf := captureLog(t)

	handler := recoverMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom in an upload handler")
	}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/upload/primuss-zip", nil))

	// Without this middleware the panic unwinds into net/http, which drops the
	// connection: the client gets no status code at all.
	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", recorder.Code)
	}

	entry := decode(t, buf)
	if entry["panic"] != "boom in an upload handler" {
		t.Errorf("panic = %v", entry["panic"])
	}
	if entry["path"] != "/upload/primuss-zip" {
		t.Errorf("path = %v", entry["path"])
	}
	if entry[obs.SkipField] != true {
		t.Errorf("%s = %v, want true", obs.SkipField, entry[obs.SkipField])
	}
}

// http.ErrAbortHandler is the documented way for a handler to abandon a
// response on purpose. net/http stays silent about it and so must we, or every
// aborted download becomes a reported bug.
func TestRecoverMiddlewarePassesErrAbortHandlerThrough(t *testing.T) {
	handler := recoverMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))

	defer func() {
		if rec := recover(); rec != http.ErrAbortHandler { //nolint:errorlint // identity, as net/http compares it
			t.Errorf("recovered %v, want it re-panicked", rec)
		}
	}()

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/download/pdf/plan", nil))
	t.Error("the panic was swallowed")
}
