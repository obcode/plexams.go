package obs

import (
	"strings"
	"testing"

	sentry "github.com/getsentry/sentry-go"
	"github.com/rs/zerolog"
)

// The DSN is fake: with an injected transport nothing is dialled, but the SDK
// insists on a well-formed one.
const testDSN = "http://0123456789abcdef0123456789abcdef@127.0.0.1:8099/1"

func TestInitWithoutADSNIsANoOp(t *testing.T) {
	defer reset()

	writer, err := Init(Config{})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if writer != nil {
		t.Error("a writer was returned without a DSN")
	}
	if Enabled() {
		t.Error("reporting is enabled without a DSN")
	}
}

// TestWriterChainScrubsALogLine is the end-to-end check the scrubber unit tests
// cannot give: a real client, a real zerolog logger, and the event as it would
// have gone on the wire.
func TestWriterChainScrubsALogLine(t *testing.T) {
	transport, logger := reporting(t, Config{DSN: testDSN, UserKey: testKey})

	logger.Error().
		Str("mtknr", testMtknr).
		Str("name", testName).
		Str("semester", "2026-SS").
		Int("ancode", 1234).
		Msgf("no registration for %s (%s)", testMtknr, testMail)

	event := onlyEvent(t, transport)

	if event.Tags["semester"] != "2026-SS" || event.Tags["ancode"] != "1234" {
		t.Errorf("the useful tags did not survive: %v", event.Tags)
	}
	for _, forbidden := range []string{"mtknr", "name"} {
		if _, ok := event.Tags[forbidden]; ok {
			t.Errorf("tag %q went out", forbidden)
		}
	}
	assertClean(t, "message", event.Message)
	if caller := event.Tags["caller"]; caller == "" {
		t.Error("no caller tag, so nothing to fingerprint on")
	} else if len(event.Fingerprint) != 1 || event.Fingerprint[0] != caller {
		t.Errorf("fingerprint = %v, want [%s]", event.Fingerprint, caller)
	}
}

func TestWriterChainIgnoresLevelsBelowError(t *testing.T) {
	transport, logger := reporting(t, Config{DSN: testDSN})

	logger.Info().Str("mtknr", testMtknr).Msg("importing")
	logger.Warn().Str("mtknr", testMtknr).Msg("no room found")

	if events := transport.Events(); len(events) != 0 {
		t.Errorf("got %d events, want none", len(events))
	}
}

func TestWriterChainHonoursTheSkipField(t *testing.T) {
	transport, logger := reporting(t, Config{DSN: testDSN})

	logger.Error().Bool(SkipField, true).Msg("reported elsewhere, with a real stack")

	if events := transport.Events(); len(events) != 0 {
		t.Errorf("got %d events, want none", len(events))
	}
}

func TestCapturePanicIsUnhandledAndCarriesAStack(t *testing.T) {
	transport, _ := reporting(t, Config{DSN: testDSN})

	func() {
		defer func() {
			if r := recover(); r != nil {
				CapturePanic(t.Context(), r)
			}
		}()
		panic("smoke " + testMtknr)
	}()

	event := onlyEvent(t, transport)

	if len(event.Exception) != 1 {
		t.Fatalf("got %d exceptions, want 1", len(event.Exception))
	}
	exception := event.Exception[0]
	assertClean(t, "exception value", exception.Value)
	if exception.Mechanism == nil || exception.Mechanism.Handled == nil || *exception.Mechanism.Handled {
		t.Errorf("mechanism = %+v, want handled=false", exception.Mechanism)
	}
	if exception.Stacktrace == nil || len(exception.Stacktrace.Frames) == 0 {
		t.Fatal("no stack trace")
	}
	// The frames must reach the function that panicked, not stop at the
	// recovery. runtime.gopanic is still on the stack while the deferred
	// function runs -- that is what makes this work, and what breaks if
	// CapturePanic is ever called one hop later.
	if !stackMentions(exception.Stacktrace, "TestCapturePanicIsUnhandledAndCarriesAStack") {
		t.Errorf("the panicking function is not in the stack: %s", frameNames(exception.Stacktrace))
	}
	// Grouping must come from that stack, not from some log line's caller.
	if event.Fingerprint != nil {
		t.Errorf("fingerprint = %v, want the default grouping", event.Fingerprint)
	}
}

func TestCapturePanicIsANoOpWhenReportingIsOff(t *testing.T) {
	defer reset()
	// Not initialised at all -- must not panic, must not dial anything.
	CapturePanic(t.Context(), "smoke")
}

// reporting initialises reporting against a mock transport and returns it
// together with a logger wired to the returned writer, as bootstrap wires it.
func reporting(t *testing.T, cfg Config) (*sentry.MockTransport, zerolog.Logger) {
	t.Helper()
	t.Cleanup(reset)

	transport := &sentry.MockTransport{}
	cfg.transport = transport

	writer, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if writer == nil {
		t.Fatal("no writer")
	}
	return transport, zerolog.New(writer).With().Caller().Timestamp().Logger()
}

func onlyEvent(t *testing.T, transport *sentry.MockTransport) *sentry.Event {
	t.Helper()
	events := transport.Events()
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	return events[0]
}

func stackMentions(st *sentry.Stacktrace, name string) bool {
	for _, f := range st.Frames {
		if strings.Contains(f.Function, name) {
			return true
		}
	}
	return false
}

func frameNames(st *sentry.Stacktrace) string {
	names := make([]string, 0, len(st.Frames))
	for _, f := range st.Frames {
		names = append(names, f.Function)
	}
	return strings.Join(names, " -> ")
}

// reset puts the package back to "not configured" so the tests do not leak
// state into each other through the global hub.
func reset() {
	enabled = false
	reportCfg.userKey = ""
	reportCfg.sendUserEmail = false
	sentry.CurrentHub().BindClient(nil)
}
