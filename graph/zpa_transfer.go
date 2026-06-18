package graph

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams"
	"github.com/rs/zerolog/log"
)

// runZPA runs a ZPA transfer (up- or download) and streams its output the same
// way the command line shows it. It refuses to run while a validation is in
// progress (the same guarantee the mutation write-guard gave before), runs the
// transfer on a background context so it finishes even if the client
// disconnects, and ends the stream with a DONE line.
func (r *subscriptionResolver) runZPA(
	ctx context.Context,
	fn func(reporter plexams.Reporter) error,
) <-chan *model.LogLine {
	ch := make(chan *model.LogLine, 256)
	reporter := newStreamReporter(ctx, ch)

	if !r.plexams.WritesAllowed() {
		go func() {
			defer close(ch)
			reporter.emit(model.LogLevelError, "error: writes are blocked while a validation is running")
			reporter.emit(model.LogLevelDone, "done")
		}()
		return ch
	}

	go func() {
		defer close(ch)
		if err := fn(reporter); err != nil {
			log.Error().Err(err).Msg("zpa transfer failed")
			reporter.emit(model.LogLevelError, "error: "+err.Error())
		}
		reporter.emit(model.LogLevelDone, "done")
	}()

	return ch
}
