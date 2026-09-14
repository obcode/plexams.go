package bootstrap

import (
	"context"
	"strings"
	"testing"
	"time"
)

// unreachableURI points at a port nothing listens on, so every attempt fails at once with
// "connection refused" instead of hanging. Port 1 is reserved and is never a database.
const unreachableURI = "postgres://plexams:plexams@127.0.0.1:1/plexams?sslmode=disable"

// The case a starting container has to survive: a database that never turns up. It must give
// up inside its budget rather than block forever, and it has to carry the underlying error out
// -- otherwise the log says "timeout" and nobody can tell a refused connection from a wrong
// password.
//
// Needs no PostgreSQL on purpose: this is the path a reboot exercises and the one that is
// awkward to reproduce against a server that works.
func TestOpenPGWaitingGivesUpAndSaysWhy(t *testing.T) {
	const budget = 1500 * time.Millisecond

	start := time.Now()
	client, err := openPGWaiting(context.Background(), unreachableURI, budget)
	elapsed := time.Since(start)

	if err == nil {
		client.Close()
		t.Fatal("openPGWaiting returned a client for a database that is not there")
	}
	if !strings.Contains(err.Error(), "not reachable within") {
		t.Errorf("error should say the budget ran out, got: %v", err)
	}
	if !strings.Contains(err.Error(), "connect") && !strings.Contains(err.Error(), "refused") {
		t.Errorf("error should carry the connection failure, got: %v", err)
	}

	// Deliberately loose: this asserts that it stops, not how precisely. A tight bound would
	// fail on a loaded CI runner and teach everyone to ignore the test.
	if elapsed > 4*budget {
		t.Errorf("took %s, well past the %s budget", elapsed, budget)
	}
}

// A shutdown during startup has to be reported as a shutdown. Calling it an unreachable
// database sends whoever reads the log to look at Postgres for what was a SIGTERM.
func TestOpenPGWaitingStopsWhenTheContextDoes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	client, err := openPGWaiting(ctx, unreachableURI, time.Minute)
	if err == nil {
		client.Close()
		t.Fatal("openPGWaiting returned a client for a cancelled context")
	}
	if !strings.Contains(err.Error(), "cancelled") {
		t.Errorf("error should name the cancellation, got: %v", err)
	}
	// It must not sit out the minute it was given.
	if time.Since(start) > 10*time.Second {
		t.Errorf("took %s; a cancelled context should return promptly", time.Since(start))
	}
}

// A URI that cannot be parsed is not something to wait for. Retrying it for 90 s would turn a
// typo in .env into a container that looks slow rather than misconfigured.
func TestOpenPGWaitingDoesNotRetryAGarbledURI(t *testing.T) {
	start := time.Now()
	client, err := openPGWaiting(context.Background(), "postgres://%zz", time.Minute)
	if err == nil {
		client.Close()
		t.Fatal("openPGWaiting accepted a URI that cannot be parsed")
	}
	if !strings.Contains(err.Error(), "cannot parse postgres uri") {
		t.Errorf("error should name the parse failure, got: %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Error("a parse failure should be immediate, not retried")
	}
}
