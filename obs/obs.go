package obs

import (
	"context"
	"fmt"
	"strings"
	"time"

	sentry "github.com/getsentry/sentry-go"
	sentryzerolog "github.com/getsentry/sentry-go/zerolog"
	"github.com/rs/zerolog"
)

// flushTimeout bounds how long a shutdown, or a log.Fatal, waits for pending
// events to reach the backend. Long enough for a slow round trip over the VPN,
// short enough that a dead monitoring host cannot hold up a restart.
const flushTimeout = 5 * time.Second

// Config is the error-reporting configuration, read from .plexams.yaml (and,
// for the DSN, from the environment) by the bootstrap package.
type Config struct {
	// DSN is the ingest URL. Empty (the default, and the whole of local
	// development) disables reporting: Init then does nothing at all and
	// returns a nil writer.
	DSN string
	// Environment separates production from a test deployment in the UI.
	Environment string
	// Release is the version this binary was built as, so an issue points at a
	// release rather than at "some time in the last three months".
	Release string
	// IgnoreErrors drops events whose message matches one of these patterns.
	// Shipped empty on purpose: filling it before a week of real traffic is
	// guessing at which noise exists.
	IgnoreErrors []string
	// SendUserEmail turns the pseudonymous user (see user.go) back into the
	// real mail address. Off by default.
	SendUserEmail bool
	// UserKey keys the pseudonym. Wired to the existing secrets.key.
	UserKey string
	// Debug makes the SDK log what it does with every event.
	Debug bool

	// transport replaces the HTTP transport. Unexported, so only this package
	// can set it: the tests use it to run a real Init and read back the events
	// that would have gone out, which is the only way to check the whole chain
	// (writer -> BeforeSend -> transport) rather than the scrubber alone.
	transport sentry.Transport
}

// reportCfg holds the parts of the configuration that the scrubber and the user
// pseudonymisation need at event time. Package state because BeforeSend is a
// bare function and the zerolog writer reaches Sentry without passing through
// any of our own call frames. Written once by Init, before the server starts,
// and only read afterwards.
var reportCfg struct {
	userKey       string
	sendUserEmail bool
}

// enabled reports whether Init actually brought up a client.
var enabled bool

// Enabled reports whether error reporting is configured. Used by callers that
// would otherwise do work only to throw it away.
func Enabled() bool { return enabled }

// Init starts error reporting and returns the zerolog writer that feeds it.
//
// The writer is the main capture path: plexams handles almost all of its errors
// by logging them, so a log line at Error level or above IS the error report.
// The caller hangs it into the logger with zerolog.MultiLevelWriter; a nil
// return means "not configured" and the logger stays exactly as it was.
//
// One client, not two. sentryzerolog.New would build a second one from its own
// ClientOptions -- with a second BeforeSend and its own buffer -- so a Flush on
// either would miss half the events and the scrubber would have to be installed
// twice. NewWithHub binds the writer to the client sentry.Init just created.
func Init(cfg Config) (zerolog.LevelWriter, error) {
	if strings.TrimSpace(cfg.DSN) == "" {
		return nil, nil
	}

	reportCfg.userKey = cfg.UserKey
	reportCfg.sendUserEmail = cfg.SendUserEmail
	scrub := scrubber{sendUserEmail: cfg.SendUserEmail}

	err := sentry.Init(sentry.ClientOptions{
		Dsn:          cfg.DSN,
		Environment:  cfg.Environment,
		Release:      cfg.Release,
		IgnoreErrors: cfg.IgnoreErrors,
		Debug:        cfg.Debug,
		BeforeSend:   scrub.scrub,
		Transport:    cfg.transport,

		// This server plans exams for a few dozen people; there is no traffic
		// volume to trace and no budget question to answer with metrics. Both
		// off means less that can carry data out by accident.
		EnableTracing:  false,
		DisableLogs:    true,
		DisableMetrics: true,

		// Only affects CaptureMessage. Events from the writer get their (useless)
		// stack from sentryzerolog, and the ones we capture bring a real one.
		AttachStacktrace: false,

		// Breadcrumbs are opt-in here: the writer adds none (WithBreadcrumbs is
		// off below), so the only ones that exist are the ones graph adds on
		// purpose. A small ceiling keeps a long-running request from carrying a
		// day's worth of them into an unrelated panic.
		MaxBreadcrumbs: 20,

		// SendDefaultPII stays false, and DataCollection then says the same
		// thing in the SDK's newer, finer-grained vocabulary -- explicitly,
		// because the SDK's fallback for a nil DataCollection is derived from
		// SendDefaultPII and would silently change with an SDK upgrade.
		//
		// Deprecated and set anyway, therefore, which is exactly what the
		// suppression below says. tallox.go/internal/obs and glabs/web/obs carry
		// the same line and the same comment; the three are meant to read alike.
		//nolint:staticcheck // SA1019: kept as the safe fallback, see above
		SendDefaultPII: false,
		DataCollection: &sentry.DataCollection{
			UserInfo:   sentry.Set(false),
			HTTPBodies: []sentry.BodyType{},
			Cookies:    &sentry.KeyValueCollectionBehavior{Mode: sentry.CollectionOff},
			// The query string of a download route is exactly where a filter
			// parameter would show up.
			QueryParams: &sentry.KeyValueCollectionBehavior{Mode: sentry.CollectionOff},
			HTTPHeaders: &sentry.HeaderCollectionConfig{
				Request: &sentry.KeyValueCollectionBehavior{
					Mode:  sentry.CollectionAllowList,
					Terms: allowedHeaderTerms(),
				},
				Response: &sentry.KeyValueCollectionBehavior{Mode: sentry.CollectionOff},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("cannot initialise error reporting: %w", err)
	}

	writer, err := sentryzerolog.NewWithHub(sentry.CurrentHub(), sentryzerolog.Options{
		// Warnings are the normal weather of a planning run (missing room,
		// unplaced exam); reporting them would bury the errors.
		Levels: []zerolog.Level{
			zerolog.ErrorLevel,
			zerolog.FatalLevel,
			zerolog.PanicLevel,
		},
		// Would turn every Info line into a breadcrumb on the next error --
		// several hundred of them during a generation run, each one carrying
		// its unfiltered fields.
		WithBreadcrumbs: false,
		FlushTimeout:    flushTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("cannot create the error-reporting log writer: %w", err)
	}

	enabled = true
	return writer, nil
}

// allowedHeaderTerms is allowedHeaders as the SDK wants it. Kept derived from
// the one map so the SDK's own header collection and the scrubber cannot drift
// apart -- the scrubber is the backstop, not a second opinion.
func allowedHeaderTerms() []string {
	terms := make([]string, 0, len(allowedHeaders))
	for h := range allowedHeaders {
		terms = append(terms, h)
	}
	return terms
}

// Flush waits for queued events to be delivered. Called on shutdown; a no-op
// when reporting is off.
func Flush() {
	if !enabled {
		return
	}
	sentry.Flush(flushTimeout)
}

// AddOperationBreadcrumb records a mutating GraphQL operation on the hub of the
// request it belongs to, so that a panic report says what was being done.
//
// Only the field name and the ancodes it touched. NOT the arguments: those are
// where the mtknr, the names and the mail addresses live (see flattenArgs in
// graph/mutation_logging.go, which builds exactly those pairs for the local
// audit log, where they belong).
//
// Nothing happens without a request-scoped hub. That is not a fallback but the
// point: breadcrumbs on the global hub would accumulate every operation of
// every user for the lifetime of the process and hang the lot onto the next
// unrelated error.
func AddOperationBreadcrumb(ctx context.Context, operation string, ancodes []int) {
	if !enabled {
		return
	}
	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		return
	}

	var data map[string]interface{}
	if len(ancodes) > 0 {
		data = map[string]interface{}{"ancodes": ancodes}
	}
	hub.AddBreadcrumb(&sentry.Breadcrumb{
		Type:     "default",
		Category: "graphql",
		Message:  operation,
		Level:    sentry.LevelInfo,
		Data:     data,
	}, nil)
}

// CapturePanic reports a recovered panic on the hub of the request it happened
// in, so it carries that request's user and breadcrumbs.
//
// It exists because the log line the caller also writes is NOT a good report of
// a panic: sentryzerolog builds its stack trace inside its own Write method, so
// the report would show zerolog's internals instead of the failing code. The
// caller therefore marks that line with SkipField and calls this instead.
//
// CALL IT FROM THE DEFERRED FUNCTION THAT RECOVERED, not from somewhere further
// along. The stack trace is taken here, and it only reaches the code that
// panicked because runtime.gopanic is still on the stack while the deferred
// function runs. One hop later the frames are gone and the report points at the
// error handler instead of at the bug.
//
// mechanism.handled is false: this is a crash that was caught, not an error
// somebody chose to report, and Sentry counts the two differently.
func CapturePanic(ctx context.Context, value any) {
	if !enabled {
		return
	}
	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		hub = sentry.CurrentHub()
	}

	handled := false
	event := sentry.NewEvent()
	event.Level = sentry.LevelFatal
	event.Message = fmt.Sprint(value)
	event.Exception = []sentry.Exception{{
		Type:       "panic",
		Value:      fmt.Sprint(value),
		Stacktrace: sentry.NewStacktrace(),
		Mechanism: &sentry.Mechanism{
			Type:    "recover",
			Handled: &handled,
		},
	}}
	hub.CaptureEvent(event)
}
