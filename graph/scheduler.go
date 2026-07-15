package graph

import (
	"context"

	"github.com/obcode/plexams.go/plexams"
	"github.com/obcode/plexams.go/plexams/scheduler"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// startScheduledSync starts the nightly ZPA/Anny auto-sync when scheduler.enabled is set.
// The scheduler owns only the timing; each fire runs plexams.RunScheduledSync (which
// guards against concurrent transfers and mails the result). ctx is cancelled on server
// shutdown, stopping the loop.
func startScheduledSync(ctx context.Context, p *plexams.Plexams) {
	if !viper.GetBool("scheduler.enabled") {
		return
	}
	cfg := plexams.ScheduledSyncConfigFromViper()
	scheduler.Start(ctx, scheduler.Config{
		Enabled: true,
		Time:    viper.GetString("scheduler.time"),
	}, func(runCtx context.Context) {
		if _, err := p.RunScheduledSync(runCtx, cfg, plexams.NewLogReporter()); err != nil {
			log.Error().Err(err).Msg("scheduled auto-sync failed")
		}
	})
}
