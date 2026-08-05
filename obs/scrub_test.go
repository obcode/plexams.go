package obs

import (
	"strings"
	"testing"

	sentry "github.com/getsentry/sentry-go"
)

// A made-up Matrikelnummer and a made-up name/address. Nothing in this file may
// come from the semester repository — see CLAUDE.md.
const (
	testMtknr = "12345678"
	testName  = "Erika Musterfrau"
	testMail  = "erika.musterfrau@hm.edu"
)

// prod is the scrubber as it is configured in production: the zero value.
var prod = scrubber{}

func TestScrubDropsEveryTagThatIsNotAllowed(t *testing.T) {
	tests := []struct {
		name string
		tag  string
		want bool // want the tag to survive
	}{
		{"caller survives, it is the fingerprint", "caller", true},
		{"semester survives", "semester", true},
		{"ancode survives", "ancode", true},
		{"mtknr is dropped", "mtknr", false},
		{"name is dropped", "name", false},
		{"email is dropped", "email", false},
		{"teacher is dropped", "teacher", false},
		{"invigilator is dropped", "invigilator", false},
		{"user is dropped", "user", false},
		// The fail-closed proof: a key that does not exist anywhere yet, i.e.
		// the log field someone will add next year without reading this file.
		{"an unknown key is dropped", "freshly_invented_field", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &sentry.Event{Tags: map[string]string{tt.tag: "irrelevant"}}
			got := prod.scrub(event, nil)
			if got == nil {
				t.Fatal("event was dropped entirely")
			}
			if _, ok := got.Tags[tt.tag]; ok != tt.want {
				t.Errorf("tag %q present = %v, want %v", tt.tag, ok, tt.want)
			}
		})
	}
}

func TestScrubRedactsFreeText(t *testing.T) {
	event := &sentry.Event{
		Message:     "student " + testMtknr + " not found, mail to " + testMail,
		Transaction: "lookup " + testMtknr,
		Exception: []sentry.Exception{{
			Type:  "error for " + testMail,
			Value: "no registration for " + testMtknr,
		}},
	}

	got := prod.scrub(event, nil)

	assertClean(t, "message", got.Message)
	assertClean(t, "transaction", got.Transaction)
	assertClean(t, "exception type", got.Exception[0].Type)
	assertClean(t, "exception value", got.Exception[0].Value)
	if want := "student [mtknr] not found, mail to [email]"; got.Message != want {
		t.Errorf("message = %q, want %q", got.Message, want)
	}
}

func TestScrubKeepsAncodesAndYearsIntact(t *testing.T) {
	// The redaction must not eat the identifiers that make a report useful:
	// ancodes are 3-5 digits, years are 4.
	event := &sentry.Event{Message: "exam 1234 in 2026 conflicts with 987"}
	got := prod.scrub(event, nil)
	if got.Message != "exam 1234 in 2026 conflicts with 987" {
		t.Errorf("message = %q, want it unchanged", got.Message)
	}
}

func TestScrubFingerprintsLogLinesOnCaller(t *testing.T) {
	event := &sentry.Event{
		Logger: zerologLogger,
		Tags:   map[string]string{"caller": "plexams/rooms.go:245"},
	}

	got := prod.scrub(event, nil)

	if len(got.Fingerprint) != 1 || got.Fingerprint[0] != "plexams/rooms.go:245" {
		t.Errorf("fingerprint = %v, want [plexams/rooms.go:245]", got.Fingerprint)
	}
}

func TestScrubLeavesCapturedEventsToDefaultGrouping(t *testing.T) {
	// A panic we captured ourselves has a real stack trace; overriding its
	// grouping with the caller of some log line would be strictly worse.
	event := &sentry.Event{Tags: map[string]string{"caller": "graph/recover.go:47"}}
	got := prod.scrub(event, nil)
	if got.Fingerprint != nil {
		t.Errorf("fingerprint = %v, want none", got.Fingerprint)
	}
}

func TestScrubHonoursTheSkipField(t *testing.T) {
	event := &sentry.Event{Tags: map[string]string{SkipField: "true", "caller": "x.go:1"}}
	if got := prod.scrub(event, nil); got != nil {
		t.Errorf("event = %v, want it dropped", got)
	}
}

func TestScrubStripsTheRequest(t *testing.T) {
	event := &sentry.Event{Request: &sentry.Request{
		Method:      "POST",
		URL:         "https://plexams.example/nta/" + testMtknr + "?mtknr=" + testMtknr,
		QueryString: "mtknr=" + testMtknr,
		Data:        `{"mtknr":"` + testMtknr + `"}`,
		Cookies:     "_oauth2_proxy=secret",
		Headers: map[string]string{
			"X-Remote-User": testMail,
			"Cookie":        "_oauth2_proxy=secret",
			"User-Agent":    "curl/8.0",
		},
		Env: map[string]string{"REMOTE_ADDR": "10.28.1.184"},
	}}

	got := prod.scrub(event, nil).Request

	if got.Method != "POST" {
		t.Errorf("method = %q, want POST", got.Method)
	}
	assertClean(t, "url", got.URL)
	if want := "https://plexams.example/nta/[mtknr]"; got.URL != want {
		t.Errorf("url = %q, want %q", got.URL, want)
	}
	if got.QueryString != "" || got.Data != "" || got.Cookies != "" || got.Env != nil {
		t.Errorf("query/data/cookies/env survived: %+v", got)
	}
	if _, ok := got.Headers["X-Remote-User"]; ok {
		t.Error("the identity header survived")
	}
	if _, ok := got.Headers["Cookie"]; ok {
		t.Error("the session cookie survived")
	}
	if got.Headers["User-Agent"] != "curl/8.0" {
		t.Errorf("user-agent = %q, want curl/8.0", got.Headers["User-Agent"])
	}
}

func TestScrubReducesTheUserToThePseudonym(t *testing.T) {
	event := &sentry.Event{User: sentry.User{
		ID:        "u_abc123",
		Email:     testMail,
		Name:      testName,
		Username:  testMail,
		IPAddress: "10.28.1.184",
		Data:      map[string]string{"mtknr": testMtknr},
	}}

	got := prod.scrub(event, nil).User

	if got.ID != "u_abc123" {
		t.Errorf("id = %q, want it kept", got.ID)
	}
	if got.Email != "" || got.Name != "" || got.Username != "" || got.IPAddress != "" || got.Data != nil {
		t.Errorf("personal fields survived: %+v", got)
	}
}

func TestScrubKeepsTheUserEmailWhenTheOperatorAsksForIt(t *testing.T) {
	event := &sentry.Event{User: sentry.User{Email: testMail}}
	got := scrubber{sendUserEmail: true}.scrub(event, nil).User
	if got.Email != testMail {
		t.Errorf("email = %q, want it kept", got.Email)
	}
}

func TestScrubFiltersBreadcrumbsAndContexts(t *testing.T) {
	event := &sentry.Event{
		Breadcrumbs: []*sentry.Breadcrumb{{
			Message: "mutation for " + testMtknr,
			Data: map[string]interface{}{
				"ancodes": []int{1234},
				"mtknr":   testMtknr,
				"name":    testName,
			},
		}},
		Contexts: map[string]sentry.Context{
			"runtime": {"name": "go"},
			"student": {"mtknr": testMtknr},
		},
	}

	got := prod.scrub(event, nil)

	crumb := got.Breadcrumbs[0]
	assertClean(t, "breadcrumb message", crumb.Message)
	if _, ok := crumb.Data["ancodes"]; !ok {
		t.Error("ancodes were dropped from the breadcrumb")
	}
	if _, ok := crumb.Data["mtknr"]; ok {
		t.Error("mtknr survived in the breadcrumb")
	}
	if _, ok := crumb.Data["name"]; ok {
		t.Error("name survived in the breadcrumb")
	}
	if _, ok := got.Contexts["runtime"]; !ok {
		t.Error("the runtime context was dropped")
	}
	if _, ok := got.Contexts["student"]; ok {
		t.Error("an unknown context survived")
	}
}

func TestScrubClearsStackFrameVariables(t *testing.T) {
	event := &sentry.Event{Exception: []sentry.Exception{{
		Stacktrace: &sentry.Stacktrace{Frames: []sentry.Frame{{
			Function: "plexams.(*Plexams).Nta",
			Vars:     map[string]interface{}{"mtknr": testMtknr},
		}}},
	}}}

	got := prod.scrub(event, nil)

	if got.Exception[0].Stacktrace.Frames[0].Vars != nil {
		t.Error("frame variables survived")
	}
}

// assertClean is the assertion this whole package exists for.
func assertClean(t *testing.T, what, got string) {
	t.Helper()
	for _, forbidden := range []string{testMtknr, testMail, "hm.edu", "@"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("%s leaked %q: %q", what, forbidden, got)
		}
	}
}
