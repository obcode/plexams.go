// Package obs is the observability layer of plexams.go: it reports errors and
// panics to a Sentry-compatible backend (GlitchTip) and owns the scrubber that
// decides what may leave this host.
//
// It sits at the top level, next to principal, for the same reason principal
// does: both graph and plexams need it, and neither may import the other.
//
// The scrubber is the security-relevant part of this package. Read scrub.go
// before adding anything that sends.
package obs

import (
	"net/url"
	"regexp"
	"strings"

	sentry "github.com/getsentry/sentry-go"
)

// zerologLogger is the value sentryzerolog stamps on every event it builds
// (its unexported `logger` constant). It is how the scrubber tells a log line
// apart from an event we captured ourselves: only the former needs the caller
// fingerprint, because only the former has a useless stack trace.
const zerologLogger = "zerolog"

// SkipField, set on a zerolog line, drops that line from the error report while
// leaving it in the local log. The emergency exit for a single noisy call site:
//
//	log.Error().Err(err).Bool(obs.SkipField, true).Msg("...")
//
// Reach for sentry.ignoreerrors in the config first — this one requires a
// deploy. It exists because some call sites are known noise (and because
// graph/recover.go logs a panic it also reports itself, with a real stack).
const SkipField = "sentry_skip"

// allowedTags is a POSITIVE list, and it is the whole point of this file.
//
// sentryzerolog turns EVERY unknown zerolog field into a Sentry tag, unfiltered.
// This code base logs mtknr 33×, name 24× and email 10×, several of them at
// Error level (e.g. plexams/rooms.go: log.Error().Str("mtknr", *mtknr)), i.e.
// exactly at the level that gets reported. A deny list would have to be kept in
// step with every new log line anyone ever writes; this direction is safe by
// itself: a field nobody thought about is dropped, not sent.
//
// Deliberately NOT here: mtknr, name, email, teacher, shortname, invigilator,
// examer, to, user. Add a key only after checking that it cannot carry a
// student's or a lecturer's identity.
var allowedTags = map[string]bool{
	"caller":    true, // the fingerprint key, and the join key back to the logs
	"semester":  true,
	"program":   true,
	"ancode":    true,
	"ancodes":   true,
	"room":      true,
	"kind":      true,
	"source":    true,
	"operation": true, // GraphQL operation NAME, never its arguments
	"field":     true,
	"day":       true,
	"host":      true,
	"status":    true,
}

// allowedHeaders are the request headers worth keeping. Same direction as the
// tags, and for a sharper reason: auth.header (X-Remote-User) carries the
// logged-in person's mail address, and Cookie carries their session.
var allowedHeaders = map[string]bool{
	"User-Agent":   true,
	"Content-Type": true,
	"Accept":       true,
}

// allowedContexts are the SDK's own runtime contexts. Nothing in plexams sets a
// context, so anything else appearing here is unaccounted for and goes.
var allowedContexts = map[string]bool{
	"device":  true,
	"os":      true,
	"runtime": true,
	"trace":   true,
}

var (
	// A Matrikelnummer is 7-10 digits. Ancodes are 3-5 digits and years are 4,
	// so this does not eat the identifiers that make a report useful. It does
	// eat epoch milliseconds and long durations — an acceptable trade in free
	// text, and the reason the local logs are JSON: there the same job is done
	// exactly, by field name (see bootstrap.reconfigureLogging).
	reMtknr = regexp.MustCompile(`\b\d{7,10}\b`)
	reEmail = regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`)
)

// redact removes the two personal identifiers that can appear in free text
// (messages, error strings, URLs) where no allow list can help. Mail addresses
// go first: an address may contain a digit run that reMtknr would otherwise
// chew up into an unrecognisable half-address.
//
// Names cannot be matched by a pattern. That is why they are handled the other
// way round, by allowedTags, and why the free-text redaction here is the second
// line of defence rather than the first.
func redact(s string) string {
	if s == "" {
		return s
	}
	s = reEmail.ReplaceAllString(s, "[email]")
	return reMtknr.ReplaceAllString(s, "[mtknr]")
}

// scrubber is the BeforeSend hook. Its zero value is the safe configuration; a
// test can therefore use scrubber{} and get production behaviour.
type scrubber struct {
	// sendUserEmail lets an operator opt out of pseudonymisation via
	// sentry.senduseremail. Off by default: with it off the address never
	// leaves this host, and "3 users affected" still works because the
	// pseudonym is stable (see user.go).
	sendUserEmail bool
}

// scrub is what every event passes through on its way out. It is fail-closed
// throughout: it copies the few things that are allowed into fresh containers
// rather than deleting the things it knows are bad, so a field, header or
// context that nobody anticipated is dropped by default.
func (s scrubber) scrub(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
	if event == nil {
		return nil
	}

	// The emergency exit, honoured before anything else.
	if _, skip := event.Tags[SkipField]; skip {
		return nil
	}

	// Group log lines by their call site.
	//
	// sentryzerolog builds the stack trace INSIDE its Write method, so the top
	// frames are identical zerolog internals for all ~640 log sites and the
	// default grouping would fold unrelated failures into one issue. The caller
	// field (bootstrap sets .With().Caller()) is the one thing that does point
	// at the failing line, so it becomes the fingerprint: at most one issue per
	// source line, each of them clickable back into the code.
	//
	// Events we captured ourselves (panics) keep the default grouping — their
	// stack traces are real.
	if caller := event.Tags["caller"]; caller != "" && event.Logger == zerologLogger {
		event.Fingerprint = []string{caller}
	}

	event.Tags = allowMap(event.Tags, allowedTags)
	event.Message = redact(event.Message)
	event.Transaction = redact(event.Transaction)

	for i := range event.Exception {
		event.Exception[i].Type = redact(event.Exception[i].Type)
		event.Exception[i].Value = redact(event.Exception[i].Value)
		scrubStacktrace(event.Exception[i].Stacktrace)
	}
	for i := range event.Threads {
		scrubStacktrace(event.Threads[i].Stacktrace)
	}

	// Breadcrumbs are not dropped wholesale because graph adds one deliberately
	// (the GraphQL field name plus the ancodes it touched, never the arguments
	// — see graph/mutation_logging.go). Everything else about them is filtered
	// the same way as an event.
	for _, b := range event.Breadcrumbs {
		if b == nil {
			continue
		}
		b.Message = redact(b.Message)
		b.Data = allowData(b.Data)
	}

	// Rebuild rather than prune: query string, POST body, cookies and the CGI
	// environment all go, and the headers survive only by name.
	if r := event.Request; r != nil {
		event.Request = &sentry.Request{
			Method:  r.Method,
			URL:     redactURL(r.URL),
			Headers: allowMap(r.Headers, allowedHeaders),
		}
	}

	if !s.sendUserEmail {
		// Keep the pseudonymous ID that user.go put there and drop everything
		// an SDK or a stray log field might have added next to it.
		event.User = sentry.User{ID: event.User.ID}
	}

	event.Contexts = allowContexts(event.Contexts)

	return event
}

// scrubStacktrace clears the per-frame local variables. The Go SDK does not
// populate them today, which is exactly why this is cheap insurance: a frame
// variable is an arbitrary value from the failing function, i.e. potentially
// the mtknr the function was called with.
func scrubStacktrace(st *sentry.Stacktrace) {
	if st == nil {
		return
	}
	for i := range st.Frames {
		st.Frames[i].Vars = nil
	}
}

// allowMap keeps the entries whose key is allowed, redacts what is left, and
// returns nil for an empty result so the field is omitted from the payload.
func allowMap(m map[string]string, allowed map[string]bool) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		if allowed[k] {
			out[k] = redact(v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// allowData is allowMap for breadcrumb data, whose values are not strings.
// Non-string values are kept as they are: they cannot be redacted, so the key
// allow list has to carry them, which is why only scalar-ish domain keys
// (ancodes, day, …) are on it.
func allowData(m map[string]interface{}) map[string]interface{} {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		if !allowedTags[k] {
			continue
		}
		if s, ok := v.(string); ok {
			out[k] = redact(s)
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// allowContexts keeps the SDK's own runtime contexts and drops the rest.
func allowContexts(c map[string]sentry.Context) map[string]sentry.Context {
	if len(c) == 0 {
		return nil
	}
	out := make(map[string]sentry.Context, len(c))
	for k, v := range c {
		if allowedContexts[k] {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// redactURL drops the query string and fragment wholesale and redacts the path.
// Nothing in plexams.go routes a mtknr through a path segment today (the GUI's
// /nta/[mtknr] page was removed for this reason), so this is belt and braces —
// but URLs are the classic way for an identifier to reappear later, and a
// download route gaining a ?mtknr= filter is a plausible Tuesday.
func redactURL(raw string) string {
	if raw == "" {
		return ""
	}
	if u, err := url.Parse(raw); err == nil {
		u.RawQuery = ""
		u.Fragment = ""
		u.User = nil
		return redact(u.String())
	}
	// Unparseable: keep everything up to the first separator, redacted.
	return redact(strings.SplitN(raw, "?", 2)[0])
}
