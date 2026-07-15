// Package scheduler runs a callback once a day at a configured local time. It is a tiny
// in-process cron replacement: no external dependency, no second process. plexams uses it
// for the nightly ZPA/Anny auto-sync (see plexams.RunScheduledSync); the callback and its
// config stay in the plexams/graph layer, this package only owns the timing loop.
package scheduler

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// Config configures the daily schedule.
type Config struct {
	// Enabled turns the scheduler on; when false Start is a no-op.
	Enabled bool
	// Time is the daily fire time as "HH:MM" in the local timezone (time.Local,
	// Europe/Berlin here). Empty defaults to "03:00".
	Time string
}

const defaultTime = "03:00"

// Start launches the daily loop in its own goroutine (unless disabled or the time is
// unparseable) and returns immediately. The loop runs until ctx is cancelled, so wiring
// ctx to the server's shutdown signal stops it cleanly. run is invoked with ctx each day
// at the configured time.
func Start(ctx context.Context, cfg Config, run func(context.Context)) {
	if !cfg.Enabled {
		return
	}
	t := strings.TrimSpace(cfg.Time)
	if t == "" {
		t = defaultTime
	}
	hh, mm, err := parseHM(t)
	if err != nil {
		log.Error().Err(err).Str("time", cfg.Time).Msg("scheduler disabled: cannot parse scheduler.time (want HH:MM)")
		return
	}
	log.Info().Str("time", fmt.Sprintf("%02d:%02d", hh, mm)).Msg("nightly auto-sync scheduler enabled")
	go loop(ctx, hh, mm, run)
}

// loop waits until the next occurrence of hh:mm, runs the callback, and repeats.
func loop(ctx context.Context, hh, mm int, run func(context.Context)) {
	for {
		now := time.Now()
		next := nextFire(now, hh, mm)
		timer := time.NewTimer(next.Sub(now))
		log.Info().Time("next", next).Msg("auto-sync scheduled")
		select {
		case <-ctx.Done():
			timer.Stop()
			log.Info().Msg("auto-sync scheduler stopped")
			return
		case <-timer.C:
			log.Info().Msg("auto-sync starting")
			run(ctx)
		}
	}
}

// nextFire returns the next local time at hh:mm strictly after now.
func nextFire(now time.Time, hh, mm int) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), hh, mm, 0, 0, time.Local)
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next
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
