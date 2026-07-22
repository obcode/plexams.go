package scheduler

import (
	"context"
	"testing"
	"time"
)

// withBerlin sets time.Local to Europe/Berlin for the duration of the test so the DST-sensitive
// fire arithmetic is exercised deterministically (main.go, which normally sets time.Local, does
// not run under `go test`).
func withBerlin(t *testing.T) {
	t.Helper()
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skipf("Europe/Berlin tzdata unavailable: %v", err)
	}
	orig := time.Local
	time.Local = berlin
	t.Cleanup(func() { time.Local = orig })
}

func TestParseHM(t *testing.T) {
	ok := []struct {
		in     string
		hh, mm int
	}{
		{"03:00", 3, 0},
		{"23:59", 23, 59},
		{"0:5", 0, 5},
		{" 7:30 ", 7, 30},
	}
	for _, c := range ok {
		hh, mm, err := parseHM(c.in)
		if err != nil || hh != c.hh || mm != c.mm {
			t.Errorf("parseHM(%q) = %d:%d, %v; want %d:%d, nil", c.in, hh, mm, err, c.hh, c.mm)
		}
	}
	bad := []string{"", "3", "3:00:00", "24:00", "03:60", "aa:bb", "-1:00"}
	for _, c := range bad {
		if _, _, err := parseHM(c); err == nil {
			t.Errorf("parseHM(%q) expected error, got nil", c)
		}
	}
}

func TestNextFire(t *testing.T) {
	// before the fire time on the same day → today
	now := time.Date(2026, 7, 15, 1, 30, 0, 0, time.Local)
	next := nextFire(now, 3, 0)
	if !next.Equal(time.Date(2026, 7, 15, 3, 0, 0, 0, time.Local)) {
		t.Errorf("nextFire before time = %v, want today 03:00", next)
	}

	// after the fire time → next day
	now = time.Date(2026, 7, 15, 5, 0, 0, 0, time.Local)
	next = nextFire(now, 3, 0)
	if !next.Equal(time.Date(2026, 7, 16, 3, 0, 0, 0, time.Local)) {
		t.Errorf("nextFire after time = %v, want tomorrow 03:00", next)
	}

	// exactly at the fire time → next day (strictly after)
	now = time.Date(2026, 7, 15, 3, 0, 0, 0, time.Local)
	next = nextFire(now, 3, 0)
	if !next.Equal(time.Date(2026, 7, 16, 3, 0, 0, 0, time.Local)) {
		t.Errorf("nextFire at time = %v, want tomorrow 03:00", next)
	}
}

// TestNextFireDST guards the civil-day arithmetic across both German DST transitions. The old
// next.Add(24*time.Hour) landed an hour off when the 24h window crossed a transition; nextFire
// must always land on the configured wall-clock time (03:00).
func TestNextFireDST(t *testing.T) {
	withBerlin(t)

	// Spring forward: night 2026-03-29 loses the 02:00–03:00 hour. From the day before,
	// the next 03:00 must be 2026-03-29 03:00 (Add(24h) would give 04:00).
	now := time.Date(2026, 3, 28, 5, 0, 0, 0, time.Local)
	next := nextFire(now, 3, 0)
	want := time.Date(2026, 3, 29, 3, 0, 0, 0, time.Local)
	if !next.Equal(want) || next.Hour() != 3 {
		t.Errorf("nextFire across spring-forward = %v (hour %d), want %v (03:00)", next, next.Hour(), want)
	}

	// Fall back: night 2026-10-25 gains an hour. The next 03:00 must be 2026-10-25 03:00
	// (Add(24h) would give 02:00).
	now = time.Date(2026, 10, 24, 5, 0, 0, 0, time.Local)
	next = nextFire(now, 3, 0)
	want = time.Date(2026, 10, 25, 3, 0, 0, 0, time.Local)
	if !next.Equal(want) || next.Hour() != 3 {
		t.Errorf("nextFire across fall-back = %v (hour %d), want %v (03:00)", next, next.Hour(), want)
	}
}

func TestPrevFire(t *testing.T) {
	// after the fire time → today's fire
	now := time.Date(2026, 7, 15, 5, 0, 0, 0, time.Local)
	if prev := prevFire(now, 3, 0); !prev.Equal(time.Date(2026, 7, 15, 3, 0, 0, 0, time.Local)) {
		t.Errorf("prevFire after time = %v, want today 03:00", prev)
	}
	// before the fire time → yesterday's fire
	now = time.Date(2026, 7, 15, 1, 30, 0, 0, time.Local)
	if prev := prevFire(now, 3, 0); !prev.Equal(time.Date(2026, 7, 14, 3, 0, 0, 0, time.Local)) {
		t.Errorf("prevFire before time = %v, want yesterday 03:00", prev)
	}
	// exactly at the fire time → yesterday's fire (strictly before)
	now = time.Date(2026, 7, 15, 3, 0, 0, 0, time.Local)
	if prev := prevFire(now, 3, 0); !prev.Equal(time.Date(2026, 7, 14, 3, 0, 0, 0, time.Local)) {
		t.Errorf("prevFire at time = %v, want yesterday 03:00", prev)
	}
}

func TestShouldCatchUp(t *testing.T) {
	now := time.Date(2026, 7, 15, 5, 0, 0, 0, time.Local) // today's 03:00 already passed

	// no persisted state (fresh deploy) → never
	if shouldCatchUp(time.Time{}, now, 3, 0) {
		t.Error("shouldCatchUp(zero) = true, want false")
	}
	// last fire was yesterday → today's 03:00 was missed → catch up
	if !shouldCatchUp(time.Date(2026, 7, 14, 3, 0, 0, 0, time.Local), now, 3, 0) {
		t.Error("shouldCatchUp(yesterday) = false, want true")
	}
	// last fire was today's 03:00 → already covered → no catch up
	if shouldCatchUp(time.Date(2026, 7, 15, 3, 0, 0, 0, time.Local), now, 3, 0) {
		t.Error("shouldCatchUp(today's fire) = true, want false")
	}
	// last fire moments ago → no catch up
	if shouldCatchUp(now.Add(-time.Minute), now, 3, 0) {
		t.Error("shouldCatchUp(just now) = true, want false")
	}
}

// newTestScheduler builds a Scheduler with injected time seams for deterministic loop tests.
func newTestScheduler(run func(context.Context, Trigger), catchUp bool, timerC <-chan time.Time) *Scheduler {
	return &Scheduler{
		ctx:      context.Background(),
		hh:       3,
		mm:       0,
		run:      run,
		catchUp:  catchUp,
		now:      func() time.Time { return time.Date(2026, 7, 15, 5, 0, 0, 0, time.Local) },
		newTimer: func(time.Duration) (<-chan time.Time, func()) { return timerC, func() {} },
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
}

func TestCatchUpRunsOnStartup(t *testing.T) {
	runs := make(chan Trigger, 2)
	never := make(chan time.Time)
	s := newTestScheduler(func(_ context.Context, tr Trigger) { runs <- tr }, true, never)
	go s.loop()

	select {
	case tr := <-runs:
		if tr != TriggerCatchUp {
			t.Errorf("startup run trigger = %q, want %q", tr, TriggerCatchUp)
		}
	case <-time.After(time.Second):
		t.Fatal("no catch-up run on startup")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown when idle = %v, want nil", err)
	}
}

func TestNoCatchUpWhenDisabled(t *testing.T) {
	runs := make(chan Trigger, 2)
	never := make(chan time.Time)
	s := newTestScheduler(func(_ context.Context, tr Trigger) { runs <- tr }, false, never)
	go s.loop()

	select {
	case tr := <-runs:
		t.Errorf("unexpected run %q without catch-up", tr)
	case <-time.After(100 * time.Millisecond):
		// good: nothing fired
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown = %v, want nil", err)
	}
}

func TestNightlyFires(t *testing.T) {
	runs := make(chan Trigger, 4)
	timerC := make(chan time.Time, 1)
	s := newTestScheduler(func(_ context.Context, tr Trigger) { runs <- tr }, false, timerC)
	// Fire the timer exactly once; subsequent iterations block on the same (now empty) channel.
	timerC <- time.Time{}
	go s.loop()

	select {
	case tr := <-runs:
		if tr != TriggerNightly {
			t.Errorf("timer run trigger = %q, want %q", tr, TriggerNightly)
		}
	case <-time.After(time.Second):
		t.Fatal("timer did not trigger a run")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown = %v, want nil", err)
	}
}

// TestShutdownDrainsInFlightRun verifies that a run in progress is not cut off immediately: it
// keeps running until the grace period elapses, then is cancelled cooperatively via its context.
func TestShutdownDrainsInFlightRun(t *testing.T) {
	started := make(chan struct{})
	released := make(chan struct{})
	var runErr error
	never := make(chan time.Time)
	s := newTestScheduler(func(ctx context.Context, _ Trigger) {
		close(started)
		<-ctx.Done() // block until Shutdown cancels the run
		runErr = ctx.Err()
		close(released)
	}, true, never)
	go s.loop()

	<-started // run is in flight

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := s.Shutdown(ctx); err == nil {
		t.Error("Shutdown while a run is in flight = nil, want timeout error")
	}

	<-released // release happens-after runErr is written
	if runErr == nil {
		t.Error("in-flight run context was not cancelled on shutdown timeout")
	}
}
