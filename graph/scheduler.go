package graph

import (
	"context"
	"strings"
	"time"

	"github.com/obcode/plexams.go/plexams"
	"github.com/obcode/plexams.go/plexams/scheduler"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// startScheduledSync starts the nightly ZPA/Anny auto-sync when scheduler.enabled is set and
// returns the scheduler handle (nil when disabled) so the server can drain it on shutdown. The
// scheduler owns only the timing; each fire runs plexams.RunScheduledSync (which guards against
// concurrent transfers and mails the result) and persists the run into the server-wide
// scheduler state. That state (the last fire time) also drives the startup catch-up: if the
// process was down across a scheduled fire, one make-up run is executed on start.
//
// The run config (recipients/source toggles) is re-read from viper on every fire, so edits to
// .plexams.yaml take effect without a restart (viper.WatchConfig, wired in bootstrap). The
// fire time and enabled flag are bound once here and still require a restart to change.
func startScheduledSync(ctx context.Context, p *plexams.Plexams) *scheduler.Scheduler {
	if !viper.GetBool("scheduler.enabled") {
		return nil
	}

	timeStr := viper.GetString("scheduler.time")
	catchUp := false
	t := timeStr
	if strings.TrimSpace(t) == "" {
		t = "03:00"
	}
	if hh, mm, err := scheduler.ParseHM(t); err == nil {
		catchUp = scheduler.ShouldCatchUp(p.SchedulerLastFire(ctx), hh, mm)
	}

	return scheduler.Start(ctx, scheduler.Config{
		Enabled: true,
		Time:    timeStr,
		CatchUp: catchUp,
	}, func(runCtx context.Context, trigger scheduler.Trigger) {
		fireAt := time.Now()
		p.RecordSchedulerFire(runCtx, fireAt, string(trigger))

		status := "ok"
		var report *plexams.SyncRunReport
		defer func() {
			if r := recover(); r != nil {
				status = "panic"
				log.Error().Interface("panic", r).Msg("scheduled auto-sync panicked")
			}
			p.SaveSchedulerOutcome(runCtx, fireAt, string(trigger), status, report)
		}()

		cfg := plexams.ScheduledSyncConfigFromViper()
		var err error
		report, err = p.RunScheduledSync(runCtx, cfg, plexams.NewLogReporter())
		if err != nil {
			log.Error().Err(err).Msg("scheduled auto-sync failed")
			status = "errors"
		} else if report != nil {
			status = report.Status()
		}
	})
}
