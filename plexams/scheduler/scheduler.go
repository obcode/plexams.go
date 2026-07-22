// Package scheduler runs a single callback once a day at a configured local time. It is a
// tiny in-process cron replacement: no external dependency, no second process. plexams uses
// it for the nightly ZPA/Anny auto-sync (see plexams.RunScheduledSync); the callback and its
// config stay in the plexams/graph layer, this package only owns the timing loop.
//
// Beyond plain daily firing it is hardened for a long-lived server:
//
//   - a panic in the callback never kills the loop goroutine (or the process): every run is
//     wrapped in recover();
//   - missed runs are caught up: if the process was down across a scheduled fire time, the
//     loop runs one catch-up on startup (Config.LastFire/CatchUp drive this — the sync is a
//     full idempotent re-import, so a single catch-up covers any number of missed nights);
//   - shutdown drains gracefully: Shutdown stops scheduling and waits (bounded) for an
//     in-flight run to finish before returning, cancelling it cooperatively on timeout.
//
// The catch-up decision (prevFire/shouldCatchUp) is kept as pure functions so it is unit
// tested without real time; persistence of the last-fire anchor lives in the graph layer.
package scheduler

import (
	"context"
	"fmt"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Trigger tells the callback why it is running.
type Trigger string

const (
	// TriggerNightly is a regular scheduled fire.
	TriggerNightly Trigger = "nightly"
	// TriggerCatchUp is the single make-up run on startup for a missed fire.
	TriggerCatchUp Trigger = "catchup"
)

// Config configures the daily schedule.
type Config struct {
	// Enabled turns the scheduler on; when false Start is a no-op and returns nil.
	Enabled bool
	// Time is the daily fire time as "HH:MM" in the local timezone (time.Local,
	// Europe/Berlin here). Empty defaults to "03:00".
	Time string
	// LastFire is the last recorded fire time (attempt, not success), used to decide a
	// catch-up. The zero time (fresh deploy) means "no catch-up".
	LastFire time.Time
	// CatchUp enables the startup catch-up; the caller normally sets it to
	// shouldCatchUp(LastFire, now, hh, mm) after loading LastFire from persistence.
	CatchUp bool
}

const defaultTime = "03:00"

// Scheduler is a running daily scheduler. Obtain one from Start and stop it with Shutdown.
type Scheduler struct {
	ctx      context.Context
	hh, mm   int
	run      func(context.Context, Trigger)
	catchUp  bool
	now      func() time.Time
	newTimer func(time.Duration) (<-chan time.Time, func())

	stop     chan struct{}
	stopOnce sync.Once
	done     chan struct{}

	mu        sync.Mutex
	cancelRun context.CancelFunc // set while a run is in flight; used by Shutdown to abort it
}

// Start validates the config and launches the daily loop in its own goroutine. It returns
// nil when the scheduler is disabled or the time is unparseable (both logged), otherwise a
// handle whose Shutdown drains the loop. run is invoked with a detached, cancellable context
// and the trigger for each fire; ctx bounds the scheduler's lifetime as a fallback stop.
func Start(ctx context.Context, cfg Config, run func(context.Context, Trigger)) *Scheduler {
	if !cfg.Enabled {
		return nil
	}
	t := strings.TrimSpace(cfg.Time)
	if t == "" {
		t = defaultTime
	}
	hh, mm, err := parseHM(t)
	if err != nil {
		log.Error().Err(err).Str("time", cfg.Time).Msg("scheduler disabled: cannot parse scheduler.time (want HH:MM)")
		return nil
	}
	log.Info().Str("time", fmt.Sprintf("%02d:%02d", hh, mm)).Bool("catchUp", cfg.CatchUp).Msg("nightly auto-sync scheduler enabled")
	s := &Scheduler{
		ctx:      ctx,
		hh:       hh,
		mm:       mm,
		run:      run,
		catchUp:  cfg.CatchUp,
		now:      time.Now,
		newTimer: realNewTimer,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	go s.loop()
	return s
}

// loop runs the optional catch-up, then waits until each next occurrence of hh:mm and fires.
func (s *Scheduler) loop() {
	defer close(s.done)

	if s.catchUp {
		log.Info().Msg("auto-sync catch-up: a scheduled run was missed while down, running now")
		s.fire(TriggerCatchUp)
	}

	for {
		now := s.now()
		next := nextFire(now, s.hh, s.mm)
		timerC, stopTimer := s.newTimer(next.Sub(now))
		log.Info().Time("next", next).Msg("auto-sync scheduled")
		select {
		case <-s.stop:
			stopTimer()
			log.Info().Msg("auto-sync scheduler stopped")
			return
		case <-s.ctx.Done():
			stopTimer()
			log.Info().Msg("auto-sync scheduler stopped (context cancelled)")
			return
		case <-timerC:
			log.Info().Msg("auto-sync starting")
			s.fire(TriggerNightly)
		}
	}
}

// fire runs the callback once, guarded against panics (a panic must never kill the loop) and
// with a detached, cancellable context so shutdown does not abort the run mid-flight — while
// still letting Shutdown cancel it cooperatively once the grace period elapses.
func (s *Scheduler) fire(trigger Trigger) {
	defer func() {
		if r := recover(); r != nil {
			log.Error().Interface("panic", r).Str("stack", string(debug.Stack())).Msg("auto-sync run panicked; loop continues")
		}
	}()

	runCtx, cancel := context.WithCancel(context.WithoutCancel(s.ctx))
	s.mu.Lock()
	s.cancelRun = cancel
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.cancelRun = nil
		s.mu.Unlock()
		cancel()
	}()

	s.run(runCtx, trigger)
}

// Shutdown stops scheduling further runs and waits for an in-flight run to finish, bounded by
// ctx. If ctx expires first it cancels the running callback cooperatively and returns
// ctx.Err(); the in-flight sync is idempotent and self-heals via the next nightly run plus the
// startup catch-up, so a cut-off run is safe. Shutdown is safe to call more than once.
func (s *Scheduler) Shutdown(ctx context.Context) error {
	s.stopOnce.Do(func() { close(s.stop) })
	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		s.mu.Lock()
		cancel := s.cancelRun
		s.mu.Unlock()
		if cancel != nil {
			log.Warn().Msg("auto-sync: shutdown grace elapsed, cancelling in-flight run")
			cancel()
		}
		return ctx.Err()
	}
}

func realNewTimer(d time.Duration) (<-chan time.Time, func()) {
	t := time.NewTimer(d)
	return t.C, func() { t.Stop() }
}

// nextFire returns the next local time at hh:mm strictly after now. The day is advanced via
// time.Date (civil day + 1), not by adding 24h, so it re-derives the zone offset per calendar
// day and stays correct across DST transitions.
func nextFire(now time.Time, hh, mm int) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), hh, mm, 0, 0, time.Local)
	if !next.After(now) {
		next = time.Date(now.Year(), now.Month(), now.Day()+1, hh, mm, 0, 0, time.Local)
	}
	return next
}

// prevFire returns the most recent local time at hh:mm strictly before now (DST-safe, civil
// day arithmetic like nextFire).
func prevFire(now time.Time, hh, mm int) time.Time {
	prev := time.Date(now.Year(), now.Month(), now.Day(), hh, mm, 0, 0, time.Local)
	if !prev.Before(now) {
		prev = time.Date(now.Year(), now.Month(), now.Day()-1, hh, mm, 0, 0, time.Local)
	}
	return prev
}

// shouldCatchUp reports whether a scheduled fire was missed while the process was down: the
// last recorded fire is older than the most recent scheduled fire time before now. A zero
// lastFire (fresh deploy, no history) never triggers a catch-up.
func shouldCatchUp(lastFire, now time.Time, hh, mm int) bool {
	if lastFire.IsZero() {
		return false
	}
	return lastFire.Before(prevFire(now, hh, mm))
}

// ShouldCatchUp is the exported catch-up decision for callers that seed Config.CatchUp from
// persisted state. lastFire is the last recorded fire (zero when none).
func ShouldCatchUp(lastFire time.Time, hh, mm int) bool {
	return shouldCatchUp(lastFire, time.Now(), hh, mm)
}

// ParseHM parses "HH:MM" into hour and minute (local wall-clock), validating the ranges. It
// lets callers derive hh/mm for ShouldCatchUp from the same config string the scheduler uses.
func ParseHM(s string) (hh, mm int, err error) {
	return parseHM(s)
}

// parseHM parses "HH:MM" into hour and minute, validating the ranges.
func parseHM(s string) (hh, mm int, err error) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid time %q, want HH:MM", s)
	}
	hh, err = strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid hour in %q: %w", s, err)
	}
	mm, err = strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid minute in %q: %w", s, err)
	}
	if hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return 0, 0, fmt.Errorf("time %q out of range (00:00–23:59)", s)
	}
	return hh, mm, nil
}
